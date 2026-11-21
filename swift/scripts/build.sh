#!/bin/bash
set -eu
cd "$(dirname "$0")/.."
export DEVELOPER_DIR="${DEVELOPER_DIR:-/Applications/Xcode.app/Contents/Developer}"
CONFIG="${1:-release}"
/usr/bin/swift build -c "$CONFIG"
BIN="$(/usr/bin/swift build -c "$CONFIG" --show-bin-path)"
APP=".build/DayTrace.app"

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Helpers" "$APP/Contents/Resources"
cp "$BIN/DayTrace" "$APP/Contents/MacOS/DayTrace"
cp "$BIN/daytrace-agent" "$APP/Contents/Helpers/daytrace-agent"

cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleIdentifier</key>
    <string>io.github.getupandwashits.daytrace</string>
    <key>CFBundleName</key>
    <string>DayTrace</string>
    <key>CFBundleDisplayName</key>
    <string>DayTrace</string>
    <key>CFBundleExecutable</key>
    <string>DayTrace</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>CFBundleShortVersionString</key>
    <string>0.1.0</string>
    <key>LSMinimumSystemVersion</key>
    <string>15.0</string>
    <key>LSUIElement</key>
    <false/>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSPrincipalClass</key>
    <string>NSApplication</string>
    <key>NSAccessibilityUsageDescription</key>
    <string>DayTrace 读取前台窗口标题和界面文字，用于生成每日活动记录。</string>
</dict>
</plist>
PLIST

/usr/bin/codesign --force --sign - "$APP/Contents/Helpers/daytrace-agent" >&2
/usr/bin/codesign --force --sign - "$APP" >&2
/usr/bin/codesign --verify --deep --strict "$APP"
printf '%s\n' "$APP"
