//go:build darwin

#import <AppKit/AppKit.h>
#include <stdlib.h>

#include "_cgo_export.h"

/* The rest prompt is a floating card, like the Swift prototype's: it comes to
 * the front without the app taking focus, stays above other windows and waits
 * for Space, Return, Escape or a click. */

@interface DTReminderPanel : NSPanel
@end

@interface DTReminderTarget : NSObject
- (void)rest:(id)sender;
- (void)later:(id)sender;
@end

static DTReminderPanel *dtReminderPanel = nil;
static DTReminderTarget *dtReminderTarget = nil;
static NSButton *dtReminderRestButton = nil;
static id dtReminderKeyMonitor = nil;
static BOOL dtReminderArmed = NO;
static NSTimeInterval dtReminderArmedAt = 0;

/* One Space while the card is up would start the rest by accident, so the
 * first press only arms it; a second press within two seconds starts. */
static void dtReminderSpace(void) {
    NSTimeInterval now = [NSDate timeIntervalSinceReferenceDate];
    if (dtReminderArmed && now - dtReminderArmedAt <= 2.0) {
        [dtReminderTarget rest:nil];
        return;
    }
    dtReminderArmed = YES;
    dtReminderArmedAt = now;
    if (dtReminderRestButton != nil) {
        dtReminderRestButton.title = @"再按一次空格";
    }
}

static void dtReminderClose(void) {
    if (dtReminderKeyMonitor != nil) {
        [NSEvent removeMonitor:dtReminderKeyMonitor];
        dtReminderKeyMonitor = nil;
    }
    if (dtReminderPanel != nil) {
        [dtReminderPanel orderOut:nil];
        [dtReminderPanel release];
        dtReminderPanel = nil;
    }
    dtReminderRestButton = nil;
    dtReminderArmed = NO;
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

@implementation DTReminderPanel

- (BOOL)canBecomeKeyWindow {
    return YES;
}

- (void)keyDown:(NSEvent *)event {
    switch (event.keyCode) {
    case 49:  /* space */
    case 36:  /* return */
    case 76:  /* enter */
        dtReminderSpace();
        return;
    case 53:  /* escape */
        [dtReminderTarget later:nil];
        return;
    }
    [super keyDown:event];
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

        DTReminderPanel *panel = [[DTReminderPanel alloc] initWithContentRect:NSMakeRect(0, 0, 440, 250)
                                                                   styleMask:(NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel)
                                                                     backing:NSBackingStoreBuffered
                                                                       defer:NO];
        panel.opaque = NO;
        panel.backgroundColor = [NSColor clearColor];
        panel.hasShadow = YES;
        panel.level = NSFloatingWindowLevel;
        panel.hidesOnDeactivate = NO;
        panel.becomesKeyOnlyIfNeeded = NO;
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
        dtReminderRestButton = restButton;
        dtReminderArmed = NO;

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

        // Space reaches the panel even when the app does not take focus.
        dtReminderKeyMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown
                                                                    handler:^NSEvent *(NSEvent *event) {
            switch (event.keyCode) {
            case 49:
            case 36:
            case 76:
                dtReminderSpace();
                return nil;
            case 53:
                [dtReminderTarget later:nil];
                return nil;
            }
            return event;
        }];

        [panel center];
        [panel makeKeyAndOrderFront:nil];
        [panel orderFrontRegardless];
        dtReminderPanel = panel;
    });
}

void dtDismissRestPrompt(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        dtReminderClose();
    });
}
