import AppKit
import SwiftUI

final class ReminderController {
    private var panel: NSPanel?

    func show(message: String, onRest: @escaping () -> Void) {
        close()
        let view = ReminderView(message: message, onRest: { [weak self] in
            self?.close()
            onRest()
        }, onLater: { [weak self] in
            self?.close()
        })
        let panel = NSPanel(
            contentRect: NSRect(x: 0, y: 0, width: 440, height: 230),
            styleMask: [.titled, .nonactivatingPanel, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        panel.title = "该休息一下了"
        panel.titlebarAppearsTransparent = true
        panel.isFloatingPanel = true
        panel.level = .floating
        panel.hidesOnDeactivate = false
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        panel.contentView = NSHostingView(rootView: view)
        panel.center()
        panel.orderFrontRegardless()
        self.panel = panel
    }

    func close() {
        panel?.close()
        panel = nil
    }
}

private struct ReminderView: View {
    let message: String
    let onRest: () -> Void
    let onLater: () -> Void

    var body: some View {
        VStack(spacing: 18) {
            Image(systemName: "cup.and.saucer.fill")
                .font(.system(size: 34))
                .foregroundStyle(.orange)
            Text("该休息一下了")
                .font(.title2.weight(.semibold))
            Text(message)
                .font(.callout)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            HStack(spacing: 12) {
                Button("稍后", action: onLater)
                    .keyboardShortcut(.cancelAction)
                Button("开始休息", action: onRest)
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
            }
        }
        .padding(28)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(.ultraThinMaterial)
    }
}
