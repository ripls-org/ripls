#!/usr/bin/env python3
"""Generate and install app icons for Flutter from SVG

This script automates the complete icon generation workflow:
1. Generates iOS (1024x1024 full-bleed) and Android (512x512 with padding) master icons
2. Copies them to the Flutter app's assets/icon/ directory
3. Runs flutter_launcher_icons to generate all platform-specific sizes

Usage:
    # First-time setup (create virtual environment and install dependencies):
    python3 -m venv venv
    ./venv/bin/pip install cairosvg pillow

    # Run the script:
    ./venv/bin/python regenerate_icons.py [path/to/logo.svg]

    # Or activate the venv first:
    source venv/bin/activate
    python regenerate_icons.py [path/to/logo.svg]
    deactivate
"""

import cairosvg
from PIL import Image
import io
import sys
import os
import subprocess
import xml.etree.ElementTree as ET
import re

def create_adaptive_svg_favicon(svg_path, dark_mode_color):
    """
    Create an adaptive SVG favicon with embedded CSS for dark mode.

    Creates a clean SVG without Inkscape metadata and namespace prefixes
    for better browser compatibility.

    Args:
        svg_path: Path to the input SVG file
        dark_mode_color: Color to use in dark mode (e.g., '#87CEEB')

    Returns:
        String containing the adaptive SVG content
    """
    tree = ET.parse(svg_path)
    root = tree.getroot()

    # Detect the original color
    old_color = None
    for style in root.findall('.//{http://www.w3.org/2000/svg}style'):
        if style.text:
            color_match = re.search(r'(?:stroke|fill):\s*(#[0-9a-fA-F]{6})', style.text)
            if color_match:
                old_color = color_match.group(1).lower()
                break

    if not old_color:
        old_color = '#3b5998'  # Default fallback

    # Build a clean SVG manually to avoid namespace issues
    # Extract viewBox
    viewbox = root.get('viewBox', '0 0 197.4 213.6')

    # Find circles
    circles = []
    for circle in root.findall('.//{http://www.w3.org/2000/svg}circle'):
        circles.append({
            'cx': circle.get('cx', '0'),
            'cy': circle.get('cy', '0'),
            'r': circle.get('r', '0')
        })

    # Find transform
    transform = ''
    for g in root.findall('.//{http://www.w3.org/2000/svg}g'):
        t = g.get('transform')
        if t:
            transform = t
            break

    # Build clean SVG
    svg_content = f'''<svg viewBox="{viewbox}" xmlns="http://www.w3.org/2000/svg">
  <style>
    .circle-stroke {{
      fill: none;
      stroke: {old_color};
      stroke-width: 12;
    }}
    @media (prefers-color-scheme: dark) {{
      .circle-stroke {{
        stroke: {dark_mode_color};
      }}
    }}
  </style>
  <g transform="{transform}">'''

    for circle in circles:
        svg_content += f'\n    <circle cx="{circle["cx"]}" cy="{circle["cy"]}" r="{circle["r"]}" class="circle-stroke"/>'

    svg_content += '\n  </g>\n</svg>'

    return svg_content

def add_padding_to_svg(svg_path, padding_percent=25):
    """
    Add padding to SVG by scaling the viewBox and expanding the canvas.

    Args:
        svg_path: Path to the input SVG file
        padding_percent: Percentage of padding to add on each side

    Returns:
        String containing the padded SVG content
    """
    # Parse the SVG
    tree = ET.parse(svg_path)
    root = tree.getroot()
    
    # Get original dimensions
    width = float(root.get('width', 400))
    height = float(root.get('height', 400))
    
    # Calculate new dimensions with padding
    padding_factor = 1 + (padding_percent / 100 * 2)  # padding on both sides
    new_width = width * padding_factor
    new_height = height * padding_factor
    
    # Get or create viewBox
    viewbox = root.get('viewBox')
    if viewbox:
        vb_parts = viewbox.split()
        vb_x, vb_y, vb_w, vb_h = map(float, vb_parts)
    else:
        vb_x, vb_y, vb_w, vb_h = 0, 0, width, height
    
    # Update SVG attributes for padded version
    root.set('width', str(new_width))
    root.set('height', str(new_height))
    root.set('viewBox', f"{vb_x} {vb_y} {vb_w * padding_factor} {vb_h * padding_factor}")
    
    # Find the main group and add a transform to center it
    # We need to offset by the padding amount
    offset = vb_w * (padding_percent / 100)
    
    for child in root:
        if child.tag.endswith('g'):  # Find first group element
            existing_transform = child.get('transform', '')
            if existing_transform:
                # Prepend the offset transform
                child.set('transform', f'translate({offset}, {offset}) {existing_transform}')
            else:
                child.set('transform', f'translate({offset}, {offset})')
            break
    
    # Convert back to string
    return ET.tostring(root, encoding='unicode')

def make_png_monochrome(png_path, color=(0, 0, 0)):
    """
    Convert a PNG to monochrome (single color silhouette with alpha).

    Args:
        png_path: Path to the PNG file to convert in-place
        color: RGB tuple for the silhouette color (default: black)
    """
    img = Image.open(png_path)
    if img.mode == 'RGBA':
        # Extract alpha channel
        alpha = img.split()[3]
        # Create solid color image with same alpha
        solid_color = Image.new('RGB', img.size, color)
        monochrome = Image.new('RGBA', img.size)
        monochrome.paste(solid_color, (0, 0))
        monochrome.putalpha(alpha)
        monochrome.save(png_path)

def svg_to_png(svg_content, output_path, width, height, is_file_path=False, white_background=False):
    """
    Convert SVG to PNG at specified dimensions.

    Args:
        svg_content: Either SVG string content or file path
        output_path: Where to save the PNG
        width: Output width in pixels
        height: Output height in pixels
        is_file_path: If True, svg_content is treated as a file path
        white_background: If True, composite the image on a white background (removes transparency)
    """
    if is_file_path:
        png_data = cairosvg.svg2png(
            url=svg_content,
            output_width=width,
            output_height=height
        )
    else:
        png_data = cairosvg.svg2png(
            bytestring=svg_content.encode('utf-8'),
            output_width=width,
            output_height=height
        )

    # If white background is requested, composite the image
    if white_background:
        img = Image.open(io.BytesIO(png_data))
        if img.mode in ('RGBA', 'LA'):
            # Create white background
            background = Image.new('RGB', img.size, (255, 255, 255))
            # Composite the image onto white background
            background.paste(img, mask=img.split()[-1] if img.mode == 'RGBA' else img.split()[1])
            # Save to bytes
            output_buffer = io.BytesIO()
            background.save(output_buffer, format='PNG')
            png_data = output_buffer.getvalue()

    # Save the PNG
    with open(output_path, 'wb') as f:
        f.write(png_data)

    print(f"Created: {output_path} ({width}x{height})")

def fix_xcode_project_pbxproj(flutter_app_dir):
    """
    Fix the Xcode project.pbxproj file after flutter_launcher_icons incorrectly
    modifies ASSETCATALOG_COMPILER_INCLUDE_ALL_APPICON_ASSETS.

    flutter_launcher_icons has a bug where it changes:
        ASSETCATALOG_COMPILER_INCLUDE_ALL_APPICON_ASSETS = YES;
    to:
        ASSETCATALOG_COMPILER_INCLUDE_ALL_APPICON_ASSETS = AppIcon;

    This function reverts that incorrect change.

    Args:
        flutter_app_dir: Path to the Flutter app root directory
    """
    pbxproj_path = os.path.join(flutter_app_dir, 'ios', 'Runner.xcodeproj', 'project.pbxproj')

    if not os.path.exists(pbxproj_path):
        print(f"Warning: Xcode project file not found at {pbxproj_path}")
        return

    # Read the file
    with open(pbxproj_path, 'r') as f:
        content = f.read()

    # Fix the incorrect setting
    # Match: ASSETCATALOG_COMPILER_INCLUDE_ALL_APPICON_ASSETS = AppIcon;
    # Replace with: ASSETCATALOG_COMPILER_INCLUDE_ALL_APPICON_ASSETS = YES;
    fixed_content = re.sub(
        r'ASSETCATALOG_COMPILER_INCLUDE_ALL_APPICON_ASSETS\s*=\s*AppIcon\s*;',
        'ASSETCATALOG_COMPILER_INCLUDE_ALL_APPICON_ASSETS = YES;',
        content
    )

    # Only write if changes were made
    if fixed_content != content:
        with open(pbxproj_path, 'w') as f:
            f.write(fixed_content)
        print(f"✓ Fixed flutter_launcher_icons bug in project.pbxproj")
        print(f"  Reverted ASSETCATALOG_COMPILER_INCLUDE_ALL_APPICON_ASSETS to YES")

def make_notification_icons(small_svg_path, flutter_app_dir):
    """
    Generate Android notification icons from simplified small logo.

    Args:
        small_svg_path: Path to simplified SVG (e.g., ripls_logo_double_thick.svg)
        flutter_app_dir: Path to the Flutter app root directory
    """
    android_res = os.path.join(flutter_app_dir, 'android', 'app', 'src', 'main', 'res')

    notification_sizes = [
        ('mdpi', 24),
        ('hdpi', 36),
        ('xhdpi', 48),
        ('xxhdpi', 72),
        ('xxxhdpi', 96)
    ]

    print("Generating Android notification icons from small logo...")
    for density, size in notification_sizes:
        drawable_dir = os.path.join(android_res, f'drawable-{density}')
        os.makedirs(drawable_dir, exist_ok=True)
        notification_icon_path = os.path.join(drawable_dir, 'ic_notification.png')

        # Generate PNG from SVG without padding
        svg_to_png(small_svg_path, notification_icon_path, size, size, is_file_path=True)

        # Convert to monochrome
        make_png_monochrome(notification_icon_path, color=(255, 255, 255))

    print(f"✓ Notification icons generated in {android_res}/drawable-*/")

def make_favicons(svg_path, website_dir):
    """
    Generate favicons for website in multiple sizes and themes.
    Creates both light and dark mode variants.

    Args:
        svg_path: Path to the SVG file (can be main logo or single-circle)
        website_dir: Path to the website directory
    """
    import json
    import shutil

    # Create assets subdirectory for icon files
    assets_dir = os.path.join(website_dir, 'assets')
    os.makedirs(assets_dir, exist_ok=True)

    print("Generating web favicons...")

    # Light blue color for dark backgrounds (good contrast on dark mode)
    light_blue = '#87CEEB'  # Sky blue - works well on dark backgrounds

    # Generate basic PNG favicons
    favicon_sizes = [
        ('favicon-16x16.png', 16),
        ('favicon-32x32.png', 32),
        ('android-chrome-192x192.png', 192),  # For web manifest
        ('android-chrome-512x512.png', 512),
    ]

    print("  Generating PNG favicons...")
    for filename, size in favicon_sizes:
        output_path = os.path.join(assets_dir, filename)
        svg_to_png(svg_path, output_path, size, size, is_file_path=True, white_background=False)

    # Generate Apple touch icon with white background (iOS design guidelines)
    apple_icon_path = os.path.join(assets_dir, 'apple-touch-icon.png')
    svg_to_png(svg_path, apple_icon_path, 180, 180, is_file_path=True, white_background=True)

    # Create adaptive SVG favicon with embedded CSS for dark mode
    print("  Generating adaptive SVG favicon with embedded dark mode CSS...")
    adaptive_svg = create_adaptive_svg_favicon(svg_path, light_blue)

    svg_adaptive_root = os.path.join(website_dir, 'icon.svg')
    svg_adaptive_assets = os.path.join(assets_dir, 'icon.svg')
    with open(svg_adaptive_root, 'w') as f:
        f.write(adaptive_svg)
    with open(svg_adaptive_assets, 'w') as f:
        f.write(adaptive_svg)
    print(f"Created: {svg_adaptive_root}")
    print(f"Created: {svg_adaptive_assets}")

    # Generate favicon.ico in root (16x16 and 32x32 combined)
    ico_path = os.path.join(website_dir, 'favicon.ico')
    img_16 = Image.open(os.path.join(assets_dir, 'favicon-16x16.png'))
    img_32 = Image.open(os.path.join(assets_dir, 'favicon-32x32.png'))
    img_16.save(ico_path, format='ICO', sizes=[(16, 16), (32, 32)], append_images=[img_32])
    print(f"Created: {ico_path}")

    # Generate site.webmanifest
    manifest = {
        "name": "Ripls",
        "short_name": "Ripls",
        "icons": [
            {
                "src": "/assets/android-chrome-192x192.png",
                "sizes": "192x192",
                "type": "image/png"
            },
            {
                "src": "/assets/android-chrome-512x512.png",
                "sizes": "512x512",
                "type": "image/png"
            }
        ],
        "theme_color": "#3b5998",
        "background_color": "#ffffff",
        "display": "standalone"
    }

    manifest_path = os.path.join(website_dir, 'site.webmanifest')
    with open(manifest_path, 'w') as f:
        json.dump(manifest, f, indent=2)
    print(f"Created: {manifest_path}")

    print(f"\n✓ Web favicons generated!")
    print(f"  - Icons: {assets_dir}/")
    print(f"  - favicon.ico: {website_dir}/")
    print(f"  - icon.svg: {website_dir}/")
    print(f"  - site.webmanifest: {website_dir}/")

def generate_icons(svg_path, flutter_app_dir, small_svg_path=None):
    """
    Generate and install app icons for Flutter from SVG files.

    This function:
    1. Generates iOS (full-bleed) and Android (padded) master icons from large logo
    2. Generates notification icons and favicons from small logo
    3. Saves them to Flutter app's assets/icon/ directory
    4. Runs flutter_launcher_icons to generate all platform-specific sizes

    Args:
        svg_path: Path to the large/detailed logo SVG file
        flutter_app_dir: Path to the Flutter app root directory
        small_svg_path: Path to small/simplified logo SVG for notifications/favicons (optional)
    """
    if small_svg_path is None:
        small_svg_path = svg_path
    if not os.path.exists(svg_path):
        print(f"Error: SVG file not found: {svg_path}")
        sys.exit(1)

    if not os.path.exists(flutter_app_dir):
        print(f"Error: Flutter app directory not found: {flutter_app_dir}")
        sys.exit(1)

    # Output to Flutter app's assets/icon directory
    output_dir = os.path.join(flutter_app_dir, 'assets', 'icon')
    os.makedirs(output_dir, exist_ok=True)

    print(f"Using SVG: {svg_path}")
    print(f"Flutter app: {flutter_app_dir}")
    print(f"Output directory: {output_dir}\n")

    # Generate iOS master icon (full-bleed with white background)
    print("Generating iOS master icon (1024x1024, full-bleed with white background)...")
    ios_output = os.path.join(output_dir, 'icon_ios_1024.png')
    svg_to_png(svg_path, ios_output, 1024, 1024, is_file_path=True, white_background=True)

    # Generate Android master icon (with safe area padding for adaptive icons)
    print("\nGenerating Android master icon (512x512, with padding)...")
    padded_svg = add_padding_to_svg(svg_path)
    android_output = os.path.join(output_dir, 'icon_android_512.png')
    svg_to_png(padded_svg, android_output, 512, 512, is_file_path=False)

    # Generate Android monochrome icon for themed icons (Android 13+)
    print("\nGenerating Android monochrome icon (512x512 for themed icons)...")
    monochrome_output = os.path.join(output_dir, 'icon_android_monochrome_512.png')
    # Generate from padded SVG, then convert to monochrome
    svg_to_png(padded_svg, monochrome_output, 512, 512, is_file_path=False)
    # Convert to monochrome (black silhouette on transparent background)
    make_png_monochrome(monochrome_output)

    print("\n✓ Master icons generated successfully!")

    # Run flutter_launcher_icons
    print("\nRunning flutter_launcher_icons to generate platform-specific sizes...")
    try:
        result = subprocess.run(
            ['flutter', 'pub', 'run', 'flutter_launcher_icons'],
            cwd=flutter_app_dir,
            check=True,
            capture_output=True,
            text=True
        )
        print(result.stdout)

        # Fix flutter_launcher_icons bug in Xcode project file
        fix_xcode_project_pbxproj(flutter_app_dir)

        # Manually install monochrome icons for themed icon support
        print("\nInstalling monochrome icons for Android themed icons...")
        android_res = os.path.join(flutter_app_dir, 'android', 'app', 'src', 'main', 'res')
        monochrome_sizes = [
            ('mdpi', 48),
            ('hdpi', 72),
            ('xhdpi', 96),
            ('xxhdpi', 144),
            ('xxxhdpi', 192)
        ]

        for density, size in monochrome_sizes:
            drawable_dir = os.path.join(android_res, f'drawable-{density}')
            os.makedirs(drawable_dir, exist_ok=True)
            mono_dest = os.path.join(drawable_dir, 'ic_launcher_monochrome.png')
            # Resize monochrome icon to appropriate size
            mono_img = Image.open(monochrome_output)
            mono_resized = mono_img.resize((size, size), Image.Resampling.LANCZOS)
            mono_resized.save(mono_dest)

        # Update adaptive icon XML to include monochrome layer
        adaptive_icon_dir = os.path.join(android_res, 'mipmap-anydpi-v26')
        adaptive_icon_path = os.path.join(adaptive_icon_dir, 'ic_launcher.xml')
        if os.path.exists(adaptive_icon_path):
            adaptive_icon_xml = '''<?xml version="1.0" encoding="utf-8"?>
<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
  <background android:drawable="@color/ic_launcher_background"/>
  <foreground android:drawable="@drawable/ic_launcher_foreground"/>
  <monochrome android:drawable="@drawable/ic_launcher_monochrome"/>
</adaptive-icon>'''
            with open(adaptive_icon_path, 'w') as f:
                f.write(adaptive_icon_xml)

        print("\n✓ All platform icons generated successfully!")
        print(f"\nIcons installed to:")
        print(f"  - iOS: {flutter_app_dir}/ios/Runner/Assets.xcassets/AppIcon.appiconset/")
        print(f"  - Android: {flutter_app_dir}/android/app/src/main/res/mipmap-*/")
        print(f"  - Android themed icons: Enabled (users on Android 13+ can customize icon colors)")

        # Generate notification icons from small logo
        print("\n")
        make_notification_icons(small_svg_path, flutter_app_dir)

        # Generate favicons for website (in content/ subdirectory, which is the web root)
        website_content_dir = os.path.join(os.path.dirname(flutter_app_dir), 'website', 'content')
        print("\n")
        make_favicons(small_svg_path, website_content_dir)
    except subprocess.CalledProcessError as e:
        print(f"\n✗ Error running flutter_launcher_icons:")
        print(e.stderr)
        sys.exit(1)
    except FileNotFoundError:
        print("\n✗ Error: flutter command not found. Make sure Flutter is installed and in your PATH.")
        sys.exit(1)

if __name__ == "__main__":
    # Default paths (relative to script location in /assets)
    default_large_svg = 'ripls_logo.svg'
    default_small_svg = 'ripls_logo_double_thick.svg'
    default_flutter_app = '../app'

    # Parse arguments: [large_logo] [small_logo] [flutter_app_dir]
    if len(sys.argv) > 1:
        large_svg_path = sys.argv[1]
    elif os.path.exists(default_large_svg):
        large_svg_path = default_large_svg
    else:
        print("Usage: python3 regenerate_icons.py [large_logo.svg] [small_logo.svg] [flutter_app_dir]")
        print(f"\nNo large logo specified and default '{default_large_svg}' not found.")
        print(f"\nDefaults:")
        print(f"  Large logo: {default_large_svg}")
        print(f"  Small logo: {default_small_svg}")
        print(f"  Flutter app: {default_flutter_app}")
        sys.exit(1)

    if len(sys.argv) > 2:
        small_svg_path = sys.argv[2]
    elif os.path.exists(default_small_svg):
        small_svg_path = default_small_svg
    else:
        print(f"Warning: Small logo '{default_small_svg}' not found. Using large logo for all icons.")
        small_svg_path = large_svg_path

    if len(sys.argv) > 3:
        flutter_app_dir = sys.argv[3]
    else:
        flutter_app_dir = default_flutter_app

    # Resolve to absolute paths
    large_svg_path = os.path.abspath(large_svg_path)
    small_svg_path = os.path.abspath(small_svg_path)
    flutter_app_dir = os.path.abspath(flutter_app_dir)

    generate_icons(large_svg_path, flutter_app_dir, small_svg_path)
