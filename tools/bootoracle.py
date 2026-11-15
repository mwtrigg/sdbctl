#!/usr/bin/env python3
"""Boot oracle: verify sdboot-tui's predictions against the real boot loader.

sdboot-tui claims to know which entries systemd-boot will show and which it
will hide. That claim can only be checked by *running systemd-boot*. This
script does it without installing an operating system:

  1. build a synthetic ESP from a table of probe entries
  2. ask sdboot-tui --check what it predicts for each probe
  3. boot the packaged systemd-boot binary under qemu + OVMF with
     `timeout menu-force`, and capture the menu as text (EDK2 redirects the
     EFI console to the serial port when there is no VGA device)
  4. compare prediction with reality, and fail on any mismatch

Requirements: qemu-system-x86_64, OVMF firmware, mkfs.vfat, sudo (loop mount),
and a packaged systemd-boot EFI binary. Nothing is written outside a temp
directory, and the host's own ESP is never touched.

Usage:
    make boot-oracle
    python3 tools/bootoracle.py --verbose
"""
import argparse
import os
import re
import shutil
import subprocess
import sys
import tempfile

# (id, title, entry body, note)
# Titles must be unique, short (the menu centres them in 80 columns) and
# upper-case so they cannot be confused with generated text.
PROBES = [
    ("good", "ORACLE-GOOD",
     "title ORACLE-GOOD\nlinux /vmlinuz-oracle\noptions root=/dev/sda1 rw\n",
     "valid kernel entry"),
    ("stale", "ORACLE-STALE",
     "title ORACLE-STALE\nlinux /vmlinuz-missing\noptions root=/dev/sda1 rw\n",
     "kernel file is gone"),
    ("chain", "ORACLE-CHAIN",
     "title ORACLE-CHAIN\nefi /EFI/Oracle/tool.efi\n",
     "efi chainload, file present"),
    ("chainbad", "ORACLE-CHAINBAD",
     "title ORACLE-CHAINBAD\nefi /EFI/Oracle/missing.efi\n",
     "efi chainload, file missing"),
    ("conflict", "ORACLE-CONFLICT",
     "title ORACLE-CONFLICT\nlinux /vmlinuz-oracle\nefi /EFI/Oracle/tool.efi\n",
     "two image keys"),
    ("ukikey", "ORACLE-UKIKEY",
     "title ORACLE-UKIKEY\nuki /EFI/Oracle/tool.efi\noptions root=/dev/sda1\n",
     "uki key (newer loaders only)"),
    ("arch", "ORACLE-ARCH",
     "title ORACLE-ARCH\nlinux /vmlinuz-oracle\narchitecture ia32\noptions root=/dev/sda1\n",
     "architecture mismatch"),
    ("noroot", "ORACLE-NOROOT",
     "title ORACLE-NOROOT\nlinux /vmlinuz-oracle\n",
     "no root= on the command line"),
    ("extrabad", "ORACLE-EXTRABAD",
     "title ORACLE-EXTRABAD\nlinux /vmlinuz-oracle\noptions root=/dev/sda1\nextra /creds/gone.cred\n",
     "extra file missing"),
]

LOADER_CANDIDATES = [
    "/usr/lib/systemd/boot/efi/systemd-bootx64.efi",
    "/usr/lib64/systemd/boot/efi/systemd-bootx64.efi",
    "/lib/systemd/boot/efi/systemd-bootx64.efi",
]

HIDE_MARKERS = (
    "hidden from boot menu",
    "ignores this entry",
    "drops this entry",
)


def die(msg):
    sys.exit(f"bootoracle: {msg}")


def require(cmd):
    if not shutil.which(cmd):
        die(f"{cmd} not found — the boot oracle needs qemu, OVMF and dosfstools")


def build_esp(work, loader_efi):
    """Write the ESP payload into work/stage and return the stage dir."""
    stage = os.path.join(work, "stage")
    os.makedirs(os.path.join(stage, "loader", "entries"))
    os.makedirs(os.path.join(stage, "EFI", "Oracle"))

    with open(os.path.join(stage, "loader", "loader.conf"), "w") as f:
        # menu-force: always show the menu and never time out, so the capture
        # is deterministic and qemu can be killed while it waits.
        f.write("timeout menu-force\ndefault good.conf\n")

    with open(os.path.join(stage, "vmlinuz-oracle"), "wb") as f:
        f.write(b"kernel")
    with open(os.path.join(stage, "EFI", "Oracle", "tool.efi"), "wb") as f:
        f.write(b"tool")

    for pid, _title, body, _note in PROBES:
        with open(os.path.join(stage, "loader", "entries", pid + ".conf"), "w") as f:
            f.write(body)

    # Install the boot loader at both the firmware entry and the fallback path.
    os.makedirs(os.path.join(stage, "EFI", "systemd"), exist_ok=True)
    os.makedirs(os.path.join(stage, "EFI", "BOOT"), exist_ok=True)
    shutil.copyfile(loader_efi, os.path.join(stage, "EFI", "systemd", "systemd-bootx64.efi"))
    shutil.copyfile(loader_efi, os.path.join(stage, "EFI", "BOOT", "BOOTX64.EFI"))
    return stage


def make_image(stage, img):
    subprocess.run(["dd", "if=/dev/zero", f"of={img}", "bs=1M", "count=64"],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run(["mkfs.vfat", "-F", "32", "-n", "EFI", img], check=True,
                   stdout=subprocess.DEVNULL)


def fill_image(stage, img, mnt):
    os.makedirs(mnt, exist_ok=True)
    subprocess.run(["sudo", "mount", "-o", "loop", img, mnt], check=True)
    try:
        subprocess.run(["sudo", "cp", "-r", stage + "/.", mnt + "/"], check=True)
        subprocess.run(["sync"], check=True)
    finally:
        subprocess.run(["sudo", "umount", mnt], check=True)


def predictions(bin_path, esp, verbose):
    """Run sdboot-tui --check and return {probe_id: predicted_hidden}."""
    out = subprocess.run([bin_path, "--check", "-esp", esp],
                         capture_output=True, text=True).stdout
    hidden = set()
    current = None
    for line in out.splitlines():
        m = re.match(r"^  ([a-z0-9_-]+)\.conf$", line)
        if m:
            current = m.group(1)
            continue
        if current and line.startswith("    ! ") and any(k in line for k in HIDE_MARKERS):
            hidden.add(current)
            current = None
    if verbose:
        print(out)
    return hidden


def _as_text(p):
    if p is None:
        return ""
    if isinstance(p, bytes):
        return p.decode("utf-8", "replace")
    return p


def find_ovmf():
    """Return (code, vars) firmware paths, tolerating distro naming differences."""
    codes = [
        "/usr/share/OVMF/OVMF_CODE_4M.fd",
        "/usr/share/OVMF/OVMF_CODE.fd",
        "/usr/share/AAVMF/AAVMF_CODE.fd",
        "/usr/lib/qemu/ovmf-x86_64-code.bin",
    ]
    for code in codes:
        if not os.path.exists(code):
            continue
        # Match the vars template to the code file's naming scheme.
        for cand in (code.replace("CODE", "VARS"), code.replace("_4M", ""),
                     "/usr/share/OVMF/OVMF_VARS.fd"):
            if os.path.exists(cand):
                return code, cand
    die("no OVMF firmware found (install the ovmf package)")


def boot_menu(qemu_timeout):
    """Boot the image and return the menu text."""
    code, vars_tpl = find_ovmf()
    work = os.environ.get("ORACLE_WORK")
    varstore = os.path.join(work, "vars.fd")
    shutil.copyfile(vars_tpl, varstore)
    cmd = [
        "qemu-system-x86_64",
        "-machine", "q35", "-m", "512", "-smp", "1", "-net", "none",
        "-nographic",
        "-drive", f"if=pflash,format=raw,readonly=on,file={code}",
        "-drive", f"if=pflash,format=raw,file={varstore}",
        "-drive", f"format=raw,file={os.path.join(work, 'esp.img')}",
    ]
    try:
        subprocess.run(cmd, capture_output=True, text=True, timeout=qemu_timeout)
        raw = ""
    except subprocess.TimeoutExpired as e:
        # Expected: menu-force means qemu is still sitting in the menu.
        raw = "".join(_as_text(p) for p in
                      (e.stdout, getattr(e, "output", None), e.stderr))
    return re.sub(r"\x1b\[[0-9;=?]*[a-zA-Z]", "", raw)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--bin", default=None, help="path to the sdboot-tui binary")
    ap.add_argument("--loader", default=None, help="systemd-boot EFI binary to boot")
    ap.add_argument("--timeout", type=int, default=75, help="qemu wall-clock budget (s)")
    ap.add_argument("--verbose", action="store_true")
    ap.add_argument("--keep", action="store_true", help="keep the work directory")
    args = ap.parse_args()

    repo = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    bin_path = args.bin or os.path.join(repo, "sdboot-tui")
    if not os.path.exists(bin_path):
        die(f"{bin_path} not found — run `make build` first")

    loader = args.loader
    if not loader:
        loader = next((p for p in LOADER_CANDIDATES if os.path.exists(p)), None)
    if not loader:
        die("no packaged systemd-boot EFI binary found (install systemd-boot-efi)")

    for c in ("qemu-system-x86_64", "mkfs.vfat", "sudo"):
        require(c)

    work = tempfile.mkdtemp(prefix="sdboot-oracle-")
    os.environ["ORACLE_WORK"] = work
    print(f"boot oracle: loader under test = {loader}")
    print(f"             work dir           = {work}")

    stage = build_esp(work, loader)
    img = os.path.join(work, "esp.img")
    make_image(stage, img)
    fill_image(stage, img, os.path.join(work, "mnt"))

    # The oracle mounts the image, so --check has to run against the mounted
    # tree rather than the stage dir. Mount it read-write for the check, then
    # unmount before booting.
    mnt = os.path.join(work, "mnt")
    subprocess.run(["sudo", "mount", "-o", "loop", img, mnt], check=True)
    try:
        predicted = predictions(bin_path, mnt, args.verbose)
    finally:
        subprocess.run(["sudo", "umount", mnt], check=True)

    menu = boot_menu(args.timeout)

    shown = {title for _pid, title, _body, _note in PROBES if title in menu}
    hidden_actual = {pid for pid, title, _body, _note in PROBES if title not in menu}

    print()
    print(f"{'probe':12} {'expectation':34} {'predicted':10} {'actual':10} verdict")
    print("-" * 84)
    failures = 0
    for pid, _title, _body, note in PROBES:
        pred = "hidden" if pid in predicted else "shown"
        act = "hidden" if pid in hidden_actual else "shown"
        ok = pred == act
        if not ok:
            failures += 1
        print(f"{pid:12} {note:34} {pred:10} {act:10} {'ok' if ok else 'MISMATCH'}")

    print()
    if failures:
        print(f"{failures} prediction(s) disagree with the real boot loader.")
        if not args.keep:
            shutil.rmtree(work, ignore_errors=True)
        sys.exit(1)
    print(f"all {len(PROBES)} predictions match systemd-boot.")
    if args.keep:
        print("work dir kept at", work)
    else:
        shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    main()
