#!/usr/bin/env python3
import argparse
import shutil
import subprocess
import tempfile
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate macOS AppIcon.appiconset PNGs from an SVG.")
    parser.add_argument(
        "--svg",
        default="app/assets/branding/keenbench_mark_v1_inverted.svg",
        help="Input SVG path (default: app/assets/branding/keenbench_mark_v1_inverted.svg)",
    )
    parser.add_argument(
        "--out-dir",
        default="app/macos/Runner/Assets.xcassets/AppIcon.appiconset",
        help="Output AppIcon.appiconset directory",
    )
    args = parser.parse_args()

    svg_path = Path(args.svg)
    out_dir = Path(args.out_dir)

    if not svg_path.is_file():
        raise SystemExit(f"SVG not found: {svg_path}")
    out_dir.mkdir(parents=True, exist_ok=True)

    if shutil.which("qlmanage") is None:
        raise SystemExit("Missing required tool: qlmanage (macOS only).")
    if shutil.which("sips") is None:
        raise SystemExit("Missing required tool: sips (macOS only).")

    sizes = [16, 32, 64, 128, 256, 512, 1024]
    with tempfile.TemporaryDirectory(prefix="keenbench-iconset-") as tmp:
        tmp_dir = Path(tmp)
        subprocess.run(
            ["qlmanage", "-t", "-s", "1024", "-o", str(tmp_dir), str(svg_path)],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        pngs = list(tmp_dir.glob("*.png"))
        if not pngs:
            raise SystemExit("qlmanage did not produce a PNG thumbnail.")
        base_png = pngs[0]

        for size in sizes:
            out_path = out_dir / f"app_icon_{size}.png"
            if size == 1024:
                shutil.copyfile(base_png, out_path)
            else:
                subprocess.run(
                    ["sips", "-z", str(size), str(size), str(base_png), "--out", str(out_path)],
                    check=True,
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                )
            print(f"Wrote {out_path} ({size}x{size})")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
