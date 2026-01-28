#!/bin/bash
set -e

# DroidRun Server Setup
# Run as root on Debian 13

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: Run as root"
    exit 1
fi

echo "=== DroidRun Server Setup ==="

# Install podman
if ! command -v podman &> /dev/null; then
    echo "[1/4] Installing podman..."
    apt install -y podman
else
    echo "[1/4] Podman already installed"
fi

# Install ADB
echo "[2/4] Installing ADB..."
apt install -y adb

# Add udev rules for Android devices
echo "[3/4] Adding udev rules..."
cat > /etc/udev/rules.d/51-android.rules << 'EOF'
# Google
SUBSYSTEM=="usb", ATTR{idVendor}=="18d1", MODE="0666", GROUP="plugdev"
# Samsung
SUBSYSTEM=="usb", ATTR{idVendor}=="04e8", MODE="0666", GROUP="plugdev"
# OnePlus
SUBSYSTEM=="usb", ATTR{idVendor}=="2a70", MODE="0666", GROUP="plugdev"
# Xiaomi
SUBSYSTEM=="usb", ATTR{idVendor}=="2717", MODE="0666", GROUP="plugdev"
# Sony
SUBSYSTEM=="usb", ATTR{idVendor}=="0fce", MODE="0666", GROUP="plugdev"
# LG
SUBSYSTEM=="usb", ATTR{idVendor}=="1004", MODE="0666", GROUP="plugdev"
# Huawei
SUBSYSTEM=="usb", ATTR{idVendor}=="12d1", MODE="0666", GROUP="plugdev"
EOF

udevadm control --reload-rules
udevadm trigger

# Build container
echo "[4/4] Building droidrun container..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
podman build -t droidrun "$SCRIPT_DIR"

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Next steps:"
echo "1. Connect Android phone via USB"
echo "2. Enable USB debugging on phone"
echo "3. Authorize computer when prompted"
echo "4. Run: adb devices"
echo "5. Start server:"
echo ""
echo "   podman run -d --name droidrun \\"
echo "     --network=host \\"
echo "     -v ~/.android:/root/.android \\"
echo "     -p 8000:8000 \\"
echo "     droidrun"
echo ""
echo "6. Test from client:"
echo "   ./droidrun-client -server http://SERVER_IP:8000 -key \$GOOGLE_API_KEY 'open settings'"
