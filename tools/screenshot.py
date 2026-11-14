#!/usr/bin/env python3
"""Render sdboot-tui screenshots for the README.

The binary is started inside a pseudo-terminal, driven with keystrokes, and
the resulting screen is emulated with pyte. The screen is then converted to
HTML and rasterised with headless Chrome.

Requirements (not vendored, install them separately):

    python3 -m venv .venv && .venv/bin/pip install pyte
    # plus a Chrome/Chromium binary named "google-chrome" (or set CHROME)

Usage:

    make screenshots                  # writes assets/*.png
    python3 tools/screenshot.py --out assets --keep-sample
"""
import argparse
import fcntl
import html
import os
import pty
import select
import shutil
import struct
import subprocess
import sys
import tempfile
import termios
import time

try:
    import pyte
except ImportError:
    sys.exit("pyte is required: pip install pyte")

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CHROME = os.environ.get("CHROME", "google-chrome")

BG = "#2e3440"
FG = "#d8dee9"

ANSI_COLORS = {
    "black": "#3b4252", "red": "#bf616a", "green": "#a3be8c", "brown": "#ebcb8b",
    "yellow": "#ebcb8b", "blue": "#81a1c1", "magenta": "#b48ead", "cyan": "#88c0d0",
    "white": "#e5e9f0", "brightblack": "#4c566a", "brightred": "#bf616a",
    "brightgreen": "#a3be8c", "brightyellow": "#ebcb8b", "brightblue": "#81a1c1",
    "brightmagenta": "#b48ead", "brightcyan": "#8fbcbb", "brightwhite": "#eceff4",
}

SAMPLE_FILES = {
    "loader/loader.conf": """# Managed by the installer — do not edit by hand
timeout 5
default arch.conf
editor yes
console-mode auto
auto-entries yes
auto-firmware yes
""",
    "loader/entries/arch.conf": """title Arch Linux
sort-key arch
version 6.12.8-arch1-1
options root=UUID=6d3376e4-fc93-4509-95ec-a21d68011da2 rw quiet
architecture x64
linux /arch/6.12.8-arch1-1/linux
initrd /arch/6.12.8-arch1-1/initrd
""",
    "loader/entries/arch-lts.conf": """title Arch Linux (LTS)
sort-key arch
version 6.6.87-arch1-1
options root=UUID=6d3376e4-fc93-4509-95ec-a21d68011da2 rw quiet
architecture x64
linux /arch/6.6.87-arch1-1/linux
initrd /arch/6.6.87-arch1-1/initrd
""",
    "loader/entries/fedora.conf": """title Fedora Linux 42
sort-key fedora
version 6.12.8-200.fc42.x86_64
options root=UUID=12345678-1234-1234-1234-123456789012 rw
architecture x64
linux /fedora/6.12.8-200.fc42.x86_64/linux
initrd /fedora/6.12.8-200.fc42.x86_64/initrd
""",
    "loader/entries/windows.conf": """# Manual chainload entry for the Windows Boot Manager
title Windows Boot Manager
version 11
efi /EFI/Microsoft/Boot/bootmgfw.efi
architecture x64
""",
    "loader/entries/old-kernel.conf": """title Arch Linux (old)
version 6.10.1-arch1-1
options root=UUID=6d3376e4-fc93-4509-95ec-a21d68011da2 rw
architecture x64
linux /arch/6.10.1-arch1-1/linux
initrd /arch/6.10.1-arch1-1/initrd
""",
}

SAMPLE_BINARIES = [
    "arch/6.12.8-arch1-1/linux",
    "arch/6.12.8-arch1-1/initrd",
    "arch/6.6.87-arch1-1/linux",
    "arch/6.6.87-arch1-1/initrd",
    "fedora/6.12.8-200.fc42.x86_64/linux",
    "fedora/6.12.8-200.fc42.x86_64/initrd",
    "EFI/Linux/fedora-42.efi",
]


def make_sample(root):
    """Create a fake ESP with a few realistic entries."""
    esp = os.path.join(root, "esp")
    for rel, content in SAMPLE_FILES.items():
        path = os.path.join(esp, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w") as f:
            f.write(content)
    for rel in SAMPLE_BINARIES:
        path = os.path.join(esp, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as f:
            f.write(b"sample")

    # Fake systemd-boot binaries with the .sdmagic version string, so the
    # diagnostics overlay has something to report.
    loader = b"MZ#### LoaderInfo: systemd-boot 259.1 ####sample"
    for rel in ("EFI/systemd/systemd-bootx64.efi", "EFI/BOOT/BOOTX64.EFI"):
        path = os.path.join(esp, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as f:
            f.write(loader)
    return esp


def color_css(value, fallback):
    if not value or value == "default":
        return fallback
    if value in ANSI_COLORS:
        return ANSI_COLORS[value]
    if len(value) == 6:
        try:
            int(value, 16)
            return "#" + value
        except ValueError:
            pass
    return fallback


class Session:
    def __init__(self, args, cols, rows):
        self.cols, self.rows = cols, rows
        self.screen = pyte.Screen(cols, rows)
        self.stream = pyte.ByteStream(self.screen)
        self.master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
        env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor",
                   LINES=str(rows), COLUMNS=str(cols))
        self.proc = subprocess.Popen(args, stdin=slave, stdout=slave, stderr=slave,
                                     env=env, close_fds=True, start_new_session=True)
        os.close(slave)

    def pump(self, seconds):
        end = time.time() + seconds
        while time.time() < end:
            r, _, _ = select.select([self.master], [], [], 0.05)
            if r:
                try:
                    data = os.read(self.master, 65536)
                except OSError:
                    break
                if not data:
                    break
                self.stream.feed(data)

    def send(self, keys, settle=0.35):
        os.write(self.master, keys.encode())
        self.pump(settle)

    def rows_html(self):
        out = []
        for y in range(self.screen.lines):
            line = self.screen.buffer[y]
            spans, cur, buf = [], None, []
            for x in range(self.screen.columns):
                cell = line[x]
                style = (cell.fg, cell.bg, cell.bold, cell.italics, cell.reverse)
                if style != cur:
                    if buf:
                        spans.append((cur, "".join(buf)))
                    buf, cur = [], style
                buf.append(cell.data)
            if buf:
                spans.append((cur, "".join(buf)))
            parts = []
            for style, text in spans:
                fg, bg, bold, italics, reverse = style
                fg_c, bg_c = color_css(fg, FG), color_css(bg, BG)
                if reverse:
                    fg_c, bg_c = bg_c, fg_c
                css = [f"color:{fg_c}"]
                if bg_c != BG:
                    css.append(f"background:{bg_c}")
                if bold:
                    css.append("font-weight:700")
                if italics:
                    css.append("font-style:italic")
                parts.append(f'<span style="{";".join(css)}">{html.escape(text)}</span>')
            out.append("".join(parts) or "&nbsp;")
        return out


def write_html(rows, path, esp):
    body = "\n".join(rows).replace(esp, "/boot")
    doc = f"""<!doctype html>
<meta charset="utf-8">
<style>
  html,body {{ margin:0; padding:0; background:{BG}; }}
  pre {{
    margin:0; padding:18px 20px; background:{BG}; color:{FG};
    font-family:"Fira Mono","DejaVu Sans Mono",monospace;
    font-size:15px; line-height:1.28; white-space:pre;
    font-variant-ligatures:none; overflow:hidden;
  }}
</style>
<pre>{body}</pre>
"""
    with open(path, "w") as f:
        f.write(doc)


def chrome_png(html_path, png_path, cols, rows):
    subprocess.run([
        CHROME, "--headless=new", "--disable-gpu", "--no-sandbox",
        "--hide-scrollbars", "--force-device-scale-factor=2",
        f"--screenshot={png_path}", f"--window-size={cols * 9 + 44},{rows * 19 + 44}",
        "--default-background-color=2e3440",
        "file://" + os.path.abspath(html_path),
    ], check=True, capture_output=True)


def shot(session, out_dir, name, esp, cols):
    html_path = os.path.join(out_dir, name + ".html")
    png_path = os.path.join(out_dir, name + ".png")
    rows = session.rows_html()
    write_html(rows, html_path, esp)
    chrome_png(html_path, png_path, cols, len(rows))
    os.remove(html_path)
    print("wrote", png_path)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=os.path.join(REPO, "assets"))
    ap.add_argument("--bin", default=os.path.join(REPO, "sdboot-tui"))
    ap.add_argument("--cols", type=int, default=120)
    ap.add_argument("--rows", type=int, default=34)
    ap.add_argument("--keep-sample", action="store_true")
    args = ap.parse_args()

    if not os.path.exists(args.bin):
        sys.exit(f"binary not found: {args.bin} (run `make build` first)")

    tmp = tempfile.mkdtemp(prefix="sdboot-shots-")
    esp = make_sample(tmp)
    os.makedirs(args.out, exist_ok=True)
    cmd = [args.bin, "-esp", esp]
    try:
        s = Session(cmd, args.cols, args.rows)
        s.pump(1.6)
        shot(s, args.out, "01_MainScreen", esp, args.cols)
        s.send("q")
        s.proc.terminate()

        s = Session(cmd, args.cols, args.rows)
        s.pump(1.6)
        s.send("\t")                      # focus the detail pane
        s.send("e")                       # edit mode
        s.send("\x1b[B\x1b[B\x1b[B")      # down to the linux field
        s.send("\x01")                    # ctrl+a: start of line
        s.send("\x0b")                    # ctrl+k: delete to end of line
        s.send("/arch")
        s.send("\t")                      # completion dropdown
        s.pump(0.5)
        shot(s, args.out, "02_EditEntryTabCompletion", esp, args.cols)
        s.send("q")
        s.proc.terminate()

        s = Session(cmd, args.cols, args.rows)
        s.pump(1.6)
        s.send("\t")
        s.send("e")
        s.send("\x01\x0b")
        s.send("Arch Linux (custom)")
        s.send("\r")                      # confirmation diff
        s.pump(0.5)
        shot(s, args.out, "03_SaveConfirmationDiff", esp, args.cols)
        s.send("q")
        s.proc.terminate()

        s = Session(cmd, args.cols, args.rows)
        s.pump(1.6)
        for _ in range(3):                # select the stale entry
            s.send("j")
        s.pump(0.4)
        shot(s, args.out, "04_EFI-Validation", esp, args.cols)
        s.send("q")
        s.proc.terminate()

        s = Session(cmd, args.cols, args.rows)
        s.pump(1.6)
        s.send("d")                       # diagnostics overlay
        s.pump(0.6)
        shot(s, args.out, "05_Diagnostics", esp, args.cols)
        s.send("q")
        s.proc.terminate()
    finally:
        if args.keep_sample:
            print("sample ESP kept at", esp)
        else:
            shutil.rmtree(tmp, ignore_errors=True)


if __name__ == "__main__":
    main()
