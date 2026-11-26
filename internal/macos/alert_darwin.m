//go:build darwin

#import <AppKit/AppKit.h>
#include <stdlib.h>

#include "_cgo_export.h"

/* The rest prompt is a floating card. It never takes keyboard focus: the user
 * can keep typing in whatever app is active, and DayTrace dismisses the card
 * on its own when focus continues or the user leaves. The only way to answer
 * it is the buttons. */

@interface DTReminderTarget : NSObject
- (void)rest:(id)sender;
- (void)later:(id)sender;
@end

static NSPanel *dtReminderPanel = nil;
static DTReminderTarget *dtReminderTarget = nil;

static void dtReminderClose(void) {
    if (dtReminderPanel != nil) {
        [dtReminderPanel orderOut:nil];
        [dtReminderPanel release];
        dtReminderPanel = nil;
    }
}

@implementation DTReminderTarget

- (void)rest:(id)sender {
    dtReminderClose();
    daytraceRestAnswer(1);
}

- (void)later:(id)sender {
    dtReminderClose();
    daytraceRestAnswer(0);
}

@end

static NSTextField *dtReminderLabel(NSString *text, CGFloat size, NSFontWeight weight, NSColor *color) {
    NSTextField *label = [NSTextField wrappingLabelWithString:text];
    label.font = [NSFont systemFontOfSize:size weight:weight];
    label.textColor = color;
    label.alignment = NSTextAlignmentCenter;
    label.maximumNumberOfLines = 0;
    return label;
}

void dtShowRestPrompt(const char *title, const char *message) {
    NSString *titleText = title != NULL ? [NSString stringWithUTF8String:title] : @"该休息一下了";
    NSString *messageText = message != NULL ? [NSString stringWithUTF8String:message] : @"";
    dispatch_async(dispatch_get_main_queue(), ^{
        dtReminderClose();
        if (dtReminderTarget == nil) {
            dtReminderTarget = [[DTReminderTarget alloc] init];
        }

        NSPanel *panel = [[NSPanel alloc] initWithContentRect:NSMakeRect(0, 0, 440, 250)
                                                     styleMask:(NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel)
                                                       backing:NSBackingStoreBuffered
                                                         defer:NO];
        panel.opaque = NO;
        panel.backgroundColor = [NSColor clearColor];
        panel.hasShadow = YES;
        panel.level = NSFloatingWindowLevel;
        panel.hidesOnDeactivate = NO;
        panel.becomesKeyOnlyIfNeeded = YES;
        panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary;

        NSVisualEffectView *card = [[NSVisualEffectView alloc] initWithFrame:panel.contentView.bounds];
        card.material = NSVisualEffectMaterialUnderWindowBackground;
        card.blendingMode = NSVisualEffectBlendingModeBehindWindow;
        card.state = NSVisualEffectStateActive;
        card.wantsLayer = YES;
        card.layer.cornerRadius = 14;
        card.layer.masksToBounds = YES;
        card.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
        [panel.contentView addSubview:card];

        NSStackView *stack = [[NSStackView alloc] init];
        stack.orientation = NSUserInterfaceLayoutOrientationVertical;
        stack.alignment = NSLayoutAttributeCenterX;
        stack.spacing = 14;
        stack.translatesAutoresizingMaskIntoConstraints = NO;
        [card addSubview:stack];

        NSImageView *icon = [[NSImageView alloc] init];
        icon.translatesAutoresizingMaskIntoConstraints = NO;
        if (@available(macOS 11.0, *)) {
            NSImage *symbol = [NSImage imageWithSystemSymbolName:@"cup.and.saucer.fill" accessibilityDescription:nil];
            icon.image = symbol;
            icon.contentTintColor = [NSColor systemOrangeColor];
        }
        [icon.widthAnchor constraintEqualToConstant:44].active = YES;
        [icon.heightAnchor constraintEqualToConstant:38].active = YES;

        NSTextField *titleLabel = dtReminderLabel(titleText, 17, NSFontWeightSemibold, [NSColor labelColor]);
        NSTextField *messageLabel = dtReminderLabel(messageText, 12, NSFontWeightRegular, [NSColor secondaryLabelColor]);

        NSButton *laterButton = [NSButton buttonWithTitle:@"稍后" target:dtReminderTarget action:@selector(later:)];
        laterButton.bezelStyle = NSBezelStyleRounded;
        laterButton.keyEquivalent = @"\033";

        NSButton *restButton = [NSButton buttonWithTitle:@"开始休息" target:dtReminderTarget action:@selector(rest:)];
        restButton.bezelStyle = NSBezelStyleRounded;
        restButton.keyEquivalent = @"\r";
        if (@available(macOS 10.12.2, *)) {
            restButton.bezelColor = [NSColor controlAccentColor];
        }

        NSStackView *buttons = [NSStackView stackViewWithViews:@[laterButton, restButton]];
        buttons.orientation = NSUserInterfaceLayoutOrientationHorizontal;
        buttons.spacing = 12;
        buttons.alignment = NSLayoutAttributeCenterY;

        [stack addArrangedSubview:icon];
        [stack addArrangedSubview:titleLabel];
        [stack addArrangedSubview:messageLabel];
        [stack setCustomSpacing:20 afterView:messageLabel];
        [stack addArrangedSubview:buttons];

        [NSLayoutConstraint activateConstraints:@[
            [stack.leadingAnchor constraintEqualToAnchor:card.leadingAnchor constant:28],
            [stack.trailingAnchor constraintEqualToAnchor:card.trailingAnchor constant:-28],
            [stack.topAnchor constraintEqualToAnchor:card.topAnchor constant:24],
            [stack.bottomAnchor constraintEqualToAnchor:card.bottomAnchor constant:-24],
            [messageLabel.widthAnchor constraintLessThanOrEqualToConstant:360],
        ]];

        [panel center];
        // No makeKeyAndOrderFront: the card must never steal keystrokes from
        // the app the user is typing in.
        [panel orderFrontRegardless];
        dtReminderPanel = panel;
    });
}

void dtDismissRestPrompt(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        dtReminderClose();
    });
}
