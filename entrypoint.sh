#!/bin/bash
set -e

echo "========================================"
echo "  DroidRun Task Queue Server"
echo "========================================"
echo ""

# Check for server authentication
if [ -z "$DROIDRUN_SERVER_KEY" ]; then
    echo "WARNING: No DROIDRUN_SERVER_KEY set - server is unauthenticated!"
    echo "         Set -e DROIDRUN_SERVER_KEY=yourkey for production use."
    echo ""
fi

# Start ADB server
adb start-server 2>/dev/null || true

# Check for connected devices
echo "Checking for Android devices..."
DEVICES=$(adb devices 2>/dev/null | grep -v "List" | grep -v "^$" || true)

if [ -z "$DEVICES" ]; then
    echo ""
    echo "WARNING: No Android devices detected!"
    echo ""
    echo "To connect your Android device:"
    echo ""
    echo "1. Enable USB Debugging on your phone:"
    echo "   Settings > Developer Options > USB Debugging"
    echo ""
    echo "2. Run container with USB access:"
    echo ""
    echo "   # Linux:"
    echo "   docker run -d --name droidrun \\"
    echo "     --privileged \\"
    echo "     -v /dev/bus/usb:/dev/bus/usb \\"
    echo "     -v ~/.android:/root/.android \\"
    echo "     -p 8000:8000 \\"
    echo "     -e DROIDRUN_SERVER_KEY=yourkey \\"
    echo "     droidrun"
    echo ""
    echo "   # Or use host network (simpler):"
    echo "   docker run -d --name droidrun \\"
    echo "     --privileged \\"
    echo "     --network=host \\"
    echo "     -v /dev/bus/usb:/dev/bus/usb \\"
    echo "     -v ~/.android:/root/.android \\"
    echo "     -e DROIDRUN_SERVER_KEY=yourkey \\"
    echo "     droidrun"
    echo ""
    echo "3. When prompted on phone, tap 'Allow' for USB debugging"
    echo ""
    echo "4. Verify connection: curl http://localhost:8000/health"
    echo ""
    echo "Starting server anyway (device can be connected later)..."
    echo ""
else
    echo ""
    echo "Found devices:"
    echo "$DEVICES" | while read line; do
        if [ -n "$line" ]; then
            SERIAL=$(echo "$line" | awk '{print $1}')
            STATUS=$(echo "$line" | awk '{print $2}')
            if [ "$STATUS" = "device" ]; then
                MODEL=$(adb -s "$SERIAL" shell getprop ro.product.model 2>/dev/null | tr -d '\r' || echo "unknown")
                echo "  ✓ $SERIAL ($MODEL) - ready"
            elif [ "$STATUS" = "unauthorized" ]; then
                echo "  ✗ $SERIAL - UNAUTHORIZED"
                echo "    → Check your phone and tap 'Allow' on the USB debugging prompt"
            else
                echo "  ? $SERIAL - $STATUS"
            fi
        fi
    done
    echo ""
fi

echo "Starting server on port ${PORT:-8000}..."
echo "Health check: http://localhost:${PORT:-8000}/health"
echo ""
echo "========================================"
echo ""

# Start the server
exec droidrun-server "${PORT:-8000}" /app/worker.py
