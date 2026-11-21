//go:build darwin

#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

#include "_cgo_export.h"

/* Viva's status items are icon-only. DayTrace shows the running countdown as
 * text, so the item, its menu and the text are owned here instead. */

typedef struct {
    char *title;
    int disabled;
    int separator;
} dt_status_entry;

@interface DayTraceStatusMenu : NSObject <NSMenuDelegate>
- (void)selected:(NSMenuItem *)sender;
@end

static NSStatusItem *dtStatusItem = nil;
static NSMenu *dtStatusMenu = nil;
static DayTraceStatusMenu *dtStatusMenuHost = nil;
static NSString *dtStatusTitle = nil;
static BOOL dtStatusMenuOpen = NO;
static NSArray *dtStatusPendingRows = nil;

static void dtStatusApplyRows(NSMenu *menu, NSArray *rows) {
    [menu removeAllItems];
    for (NSDictionary *row in rows) {
        if ([row[@"separator"] boolValue]) {
            [menu addItem:[NSMenuItem separatorItem]];
            continue;
        }
        NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:row[@"title"]
                                                      action:@selector(selected:)
                                               keyEquivalent:@""];
        item.target = dtStatusMenuHost;
        item.tag = [row[@"index"] integerValue];
        item.enabled = ![row[@"disabled"] boolValue];
        [menu addItem:item];
        [item release];
    }
}

@implementation DayTraceStatusMenu

- (void)selected:(NSMenuItem *)sender {
    daytraceStatusAction((int)sender.tag);
}

- (void)menuWillOpen:(NSMenu *)menu {
    dtStatusMenuOpen = YES;
}

- (void)menuDidClose:(NSMenu *)menu {
    dtStatusMenuOpen = NO;
    if (dtStatusPendingRows != nil) {
        NSArray *rows = dtStatusPendingRows;
        dtStatusPendingRows = nil;
        dtStatusApplyRows(menu, rows);
        [rows release];
    }
}

@end

void dtStatusItemInstall(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (dtStatusItem != nil) {
            return;
        }
        dtStatusMenuHost = [[DayTraceStatusMenu alloc] init];
        dtStatusMenu = [[NSMenu alloc] initWithTitle:@"DayTrace"];
        dtStatusMenu.autoenablesItems = NO;
        dtStatusMenu.delegate = dtStatusMenuHost;
        dtStatusItem = [[[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength] retain];
        NSStatusBarButton *button = dtStatusItem.button;
        button.title = dtStatusTitle != nil ? dtStatusTitle : @"";
        // A fixed digit width keeps the item from shifting every second.
        button.font = [NSFont monospacedDigitSystemFontOfSize:12 weight:NSFontWeightRegular];
        button.imagePosition = NSNoImage;
        button.toolTip = @"DayTrace";
        dtStatusItem.menu = dtStatusMenu;
    });
}

void dtStatusItemSetTitle(const char *title) {
    NSString *text = title != NULL ? [NSString stringWithUTF8String:title] : @"";
    dispatch_async(dispatch_get_main_queue(), ^{
        [dtStatusTitle release];
        dtStatusTitle = [text copy];
        if (dtStatusItem != nil) {
            dtStatusItem.button.title = dtStatusTitle;
        }
    });
}

void dtStatusItemSetMenu(dt_status_entry *entries, int count) {
    NSMutableArray *rows = [NSMutableArray arrayWithCapacity:(NSUInteger)count];
    for (int index = 0; index < count; index++) {
        dt_status_entry entry = entries[index];
        NSString *title = entry.title != NULL ? [NSString stringWithUTF8String:entry.title] : @"";
        [rows addObject:@{
            @"title": title,
            @"disabled": @(entry.disabled != 0),
            @"separator": @(entry.separator != 0),
            @"index": @(index),
        }];
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        if (dtStatusMenu == nil) {
            return;
        }
        if (dtStatusMenuOpen) {
            [dtStatusPendingRows release];
            dtStatusPendingRows = [rows copy];
            return;
        }
        dtStatusApplyRows(dtStatusMenu, rows);
    });
}
