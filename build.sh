#!/bin/bash
set -e

echo "🔨 Building Go Links for Raspberry Pi..."
echo "📦 Target: ARMv6 (Raspberry Pi Model B / Zero)"

# Build for ARMv6 (Raspberry Pi Model B)
echo "🔧 Compiling..."
GOOS=linux GOARCH=arm GOARM=6 go build -ldflags="-s -w" -o golink main.go

# Make it executable
chmod +x golink

# Get file size
SIZE=$(du -h golink | cut -f1)

echo ""
echo "✅ Build complete!"
echo "   Binary: ./golink ($SIZE)"
echo "   Target: Raspberry Pi (ARMv6)"
echo ""
echo "📝 Next steps:"
echo "   1. Transfer to Raspberry Pi: scp golink golink.service pi@<PI_IP>:/home/pi/"
echo "   2. Follow installation steps in README.md"


