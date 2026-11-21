#!/usr/bin/env bash
# 构建 DayTrace.app 和调试 CLI。
#
# 用法：
#   ./scripts/build.sh              # 构建 build/DayTrace.app（ad-hoc 签名）
#   ./scripts/build.sh --install    # 构建后复制到 /Applications 并重新签名
#
# 说明：改动代码后辅助功能权限需要重新授予，因为 ad-hoc 签名的可执行文件换了。
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ $# -gt 1 || ( $# -eq 1 && "$1" != "--install" ) ]]; then
    echo "用法：$0 [--install]" >&2
    exit 2
fi

# Go 1.26 is the last toolchain with macOS 12 support.
export GOTOOLCHAIN=go1.26.11

OUT="$PWD/build/DayTrace.app"
mkdir -p build .build

echo "==> go test"
go test ./...

echo "==> go vet"
go vet ./...
go tool viva vet

echo "==> daytrace-agent"
go build -o .build/daytrace-agent ./cmd/daytrace-agent

echo "==> viva package"
go tool viva package --format app --sign-identity - -o "$OUT"

# 分发 .app 时附带许可证与第三方声明（Viva 是 Apache-2.0，需要保留 NOTICE）。
RESOURCES="$OUT/Contents/Resources"
VIVA="$(go list -m -f '{{.Dir}}' github.com/vivaui/viva)"
cp THIRD_PARTY_NOTICES.md LICENSE LICENSES/Apache-2.0.txt "$RESOURCES/"
cp "$VIVA/THIRD_PARTY_NOTICES.md" "$RESOURCES/Viva-Third-Party-Notices.md"
cp "$(go env GOROOT)/LICENSE" "$RESOURCES/Go-License.txt"

# 菜单栏应用：不出现在 Dock，也没有全局应用菜单。
PLIST="$OUT/Contents/Info.plist"
if ! /usr/libexec/PlistBuddy -c 'Print :LSUIElement' "$PLIST" >/dev/null 2>&1; then
    /usr/libexec/PlistBuddy -c 'Add :LSUIElement bool true' "$PLIST"
fi
codesign --force --sign - "$OUT"
codesign --verify --deep --strict "$OUT"

if [[ "${1:-}" == "--install" ]]; then
    echo "==> install to /Applications"
    TARGET="/Applications/DayTrace.app"
    STAGE="$(mktemp -d /Applications/.DayTrace-install.XXXXXXXX)"
    trap 'if [[ ! -e "$STAGE/Previous.app" ]]; then rm -rf "$STAGE"; fi' EXIT
    ditto "$OUT" "$STAGE/DayTrace.app"
    codesign --verify --deep --strict "$STAGE/DayTrace.app"
    if [[ -e "$TARGET" ]]; then
        mv "$TARGET" "$STAGE/Previous.app"
    fi
    if ! mv "$STAGE/DayTrace.app" "$TARGET"; then
        if [[ -e "$STAGE/Previous.app" ]] && ! mv "$STAGE/Previous.app" "$TARGET"; then
            echo "旧版本保留在：$STAGE/Previous.app" >&2
        fi
        exit 1
    fi
    rm -rf "$STAGE"
    echo "已安装：/Applications/DayTrace.app"
    echo "首次运行或重新构建后，需要在「系统设置 → 隐私与安全性 → 辅助功能」里重新勾选 DayTrace。"
fi

echo "完成：$OUT"
