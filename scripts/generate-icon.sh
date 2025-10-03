#!/bin/bash

# Generate PNG icon from SVG for Unraid plugin
# This script converts the SVG icon to multiple PNG sizes needed for Unraid

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
SVG_FILE="$PROJECT_ROOT/meta/plugin/images/unraid-management-agent.svg"
IMAGES_DIR="$PROJECT_ROOT/meta/plugin/images"

echo "========================================="
echo "Unraid Plugin Icon Generator"
echo "========================================="
echo ""

# Check if SVG exists
if [ ! -f "$SVG_FILE" ]; then
    echo "❌ Error: SVG file not found at $SVG_FILE"
    exit 1
fi

echo "✅ Found SVG file: $SVG_FILE"
echo ""

# Check for available conversion tools
HAS_IMAGEMAGICK=false
HAS_RSVG=false
HAS_INKSCAPE=false

if command -v convert &> /dev/null; then
    HAS_IMAGEMAGICK=true
    echo "✅ ImageMagick (convert) available"
elif command -v magick &> /dev/null; then
    HAS_IMAGEMAGICK=true
    echo "✅ ImageMagick (magick) available"
fi

if command -v rsvg-convert &> /dev/null; then
    HAS_RSVG=true
    echo "✅ rsvg-convert available"
fi

if command -v inkscape &> /dev/null; then
    HAS_INKSCAPE=true
    echo "✅ Inkscape available"
fi

echo ""

# Function to convert using ImageMagick
convert_with_imagemagick() {
    local size=$1
    local output=$2
    
    if command -v convert &> /dev/null; then
        convert -background none -density 300 "$SVG_FILE" -resize ${size}x${size} "$output"
    else
        magick convert -background none -density 300 "$SVG_FILE" -resize ${size}x${size} "$output"
    fi
}

# Function to convert using rsvg
convert_with_rsvg() {
    local size=$1
    local output=$2
    rsvg-convert -w $size -h $size "$SVG_FILE" > "$output"
}

# Function to convert using Inkscape
convert_with_inkscape() {
    local size=$1
    local output=$2
    inkscape "$SVG_FILE" --export-type=png --export-width=$size --export-height=$size --export-filename="$output"
}

# Generate PNGs
echo "Generating PNG icons..."
echo ""

SIZES=(48 64 128)
NAMES=("unraid-management-agent-48.png" "unraid-management-agent.png" "unraid-management-agent-128.png")

for i in "${!SIZES[@]}"; do
    size=${SIZES[$i]}
    name=${NAMES[$i]}
    output="$IMAGES_DIR/$name"
    
    echo "Creating ${size}x${size} PNG: $name"
    
    if [ "$HAS_IMAGEMAGICK" = true ]; then
        convert_with_imagemagick $size "$output"
        echo "  ✅ Created using ImageMagick"
    elif [ "$HAS_RSVG" = true ]; then
        convert_with_rsvg $size "$output"
        echo "  ✅ Created using rsvg-convert"
    elif [ "$HAS_INKSCAPE" = true ]; then
        convert_with_inkscape $size "$output"
        echo "  ✅ Created using Inkscape"
    else
        echo "  ⚠️  No conversion tool available"
        echo ""
        echo "========================================="
        echo "MANUAL CONVERSION REQUIRED"
        echo "========================================="
        echo ""
        echo "No SVG conversion tools found. Please install one of:"
        echo ""
        echo "  MacOS:"
        echo "    brew install imagemagick"
        echo "    brew install librsvg"
        echo ""
        echo "  Linux (Debian/Ubuntu):"
        echo "    apt-get install imagemagick"
        echo "    apt-get install librsvg2-bin"
        echo ""
        echo "Or use an online converter:"
        echo "  1. Go to: https://cloudconvert.com/svg-to-png"
        echo "  2. Upload: $SVG_FILE"
        echo "  3. Set size to ${size}x${size}"
        echo "  4. Download as: $name"
        echo "  5. Save to: $IMAGES_DIR"
        echo ""
        exit 1
    fi
    
    # Verify the file was created
    if [ -f "$output" ]; then
        file_size=$(ls -lh "$output" | awk '{print $5}')
        echo "  📦 Size: $file_size"
    else
        echo "  ❌ Failed to create PNG"
        exit 1
    fi
    
    echo ""
done

echo "========================================="
echo "✅ Icon Generation Complete!"
echo "========================================="
echo ""
echo "Generated files:"
for i in "${!SIZES[@]}"; do
    name=${NAMES[$i]}
    if [ -f "$IMAGES_DIR/$name" ]; then
        echo "  ✅ $name"
    fi
done
echo ""
echo "Next steps:"
echo "  1. Review icons: ls -lh meta/plugin/images/*.png"
echo "  2. Rebuild plugin: make package"
echo "  3. Deploy: ./scripts/deploy-to-unraid.sh 192.168.20.21"
echo ""
