//go:build darwin

#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#include "_cgo_export.h"

int dtScreenLocked(void);

static void dtObservePresence(NSNotificationCenter *center, NSString *notification, const char *reason, int suspended) {
    [center addObserverForName:notification object:nil queue:[NSOperationQueue mainQueue]
                   usingBlock:^(NSNotification *note) {
        daytracePresenceChanged((char *)reason, suspended);
    }];
}

void dtInstallPresence(void) {
    static BOOL installed = NO;
    if (installed) {
        return;
    }
    installed = YES;

    NSNotificationCenter *workspace = NSWorkspace.sharedWorkspace.notificationCenter;
    dtObservePresence(workspace, NSWorkspaceWillSleepNotification, "systemSleep", 1);
    dtObservePresence(workspace, NSWorkspaceDidWakeNotification, "systemSleep", 0);
    dtObservePresence(workspace, NSWorkspaceScreensDidSleepNotification, "displaySleep", 1);
    dtObservePresence(workspace, NSWorkspaceScreensDidWakeNotification, "displaySleep", 0);

    NSDistributedNotificationCenter *session = [NSDistributedNotificationCenter defaultCenter];
    dtObservePresence(session, @"com.apple.screenIsLocked", "lock", 1);
    dtObservePresence(session, @"com.apple.screenIsUnlocked", "lock", 0);
    if (dtScreenLocked()) {
        daytracePresenceChanged("lock", 1);
    }
    if (CGDisplayIsAsleep(CGMainDisplayID())) {
        daytracePresenceChanged("displaySleep", 1);
    }
}
