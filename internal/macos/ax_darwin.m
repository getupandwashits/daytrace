//go:build darwin

#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreGraphics/CoreGraphics.h>
#import <dispatch/dispatch.h>
#include <stdlib.h>
#include <string.h>

int dtAXTrusted(void) {
    return AXIsProcessTrusted() ? 1 : 0;
}

int dtAXPrompt(void) {
    CFStringRef keys[] = { kAXTrustedCheckOptionPrompt };
    CFBooleanRef values[] = { kCFBooleanTrue };
    CFDictionaryRef options = CFDictionaryCreate(NULL, (const void **)keys, (const void **)values, 1,
                                                 &kCFTypeDictionaryKeyCallBacks,
                                                 &kCFTypeDictionaryValueCallBacks);
    if (options == NULL) {
        return 0;
    }
    int result = AXIsProcessTrustedWithOptions(options) ? 1 : 0;
    CFRelease(options);
    return result;
}

double dtIdleSeconds(void) {
    return CGEventSourceSecondsSinceLastEventType(kCGEventSourceStateCombinedSessionState,
                                                  kCGAnyInputEventType);
}

int dtScreenLocked(void) {
    CFDictionaryRef session = CGSessionCopyCurrentDictionary();
    if (session == NULL) {
        return 0;
    }
    CFBooleanRef locked = CFDictionaryGetValue(session, CFSTR("CGSSessionScreenIsLocked"));
    int value = (locked != NULL && CFBooleanGetValue(locked)) ? 1 : 0;
    CFRelease(session);
    return value;
}

CFTypeRef dtAXCreateApplication(pid_t pid) {
    return AXUIElementCreateApplication(pid);
}

void dtAXSetTimeout(CFTypeRef element, float seconds) {
    if (element != NULL) {
        AXUIElementSetMessagingTimeout((AXUIElementRef)element, seconds);
    }
}

CFTypeRef dtAXCopyAttribute(CFTypeRef element, const char *name) {
    if (element == NULL) {
        return NULL;
    }
    CFStringRef key = CFStringCreateWithCString(NULL, name, kCFStringEncodingUTF8);
    if (key == NULL) {
        return NULL;
    }
    CFTypeRef value = NULL;
    AXUIElementCopyAttributeValue((AXUIElementRef)element, key, &value);
    CFRelease(key);
    return value;
}

static char *copyCFString(CFStringRef string) {
    if (string == NULL) {
        return NULL;
    }
    CFIndex length = CFStringGetLength(string);
    CFIndex maxSize = CFStringGetMaximumSizeForEncoding(length, kCFStringEncodingUTF8) + 1;
    char *buffer = malloc((size_t)maxSize);
    if (buffer == NULL) {
        return NULL;
    }
    if (!CFStringGetCString(string, buffer, maxSize, kCFStringEncodingUTF8)) {
        free(buffer);
        return NULL;
    }
    return buffer;
}

char *dtAXCopyString(CFTypeRef element, const char *name) {
    CFTypeRef value = dtAXCopyAttribute(element, name);
    if (value == NULL) {
        return NULL;
    }
    char *result = NULL;
    if (CFGetTypeID(value) == CFStringGetTypeID()) {
        result = copyCFString((CFStringRef)value);
    }
    CFRelease(value);
    return result;
}

int dtAXIsArray(CFTypeRef value) {
    return (value != NULL && CFGetTypeID(value) == CFArrayGetTypeID()) ? 1 : 0;
}

long dtAXArrayCount(CFTypeRef value) {
    if (!dtAXIsArray(value)) {
        return 0;
    }
    return (long)CFArrayGetCount((CFArrayRef)value);
}

CFTypeRef dtAXArrayItem(CFTypeRef value, long index) {
    if (!dtAXIsArray(value)) {
        return NULL;
    }
    CFArrayRef array = (CFArrayRef)value;
    if (index < 0 || index >= CFArrayGetCount(array)) {
        return NULL;
    }
    return CFArrayGetValueAtIndex(array, (CFIndex)index);
}

int dtAXIsElement(CFTypeRef value) {
    return (value != NULL && CFGetTypeID(value) == AXUIElementGetTypeID()) ? 1 : 0;
}

char *dtAXURLCopyString(CFTypeRef value) {
    if (value == NULL) {
        return NULL;
    }
    CFTypeID type = CFGetTypeID(value);
    if (type == CFURLGetTypeID()) {
        return copyCFString(CFURLGetString((CFURLRef)value));
    }
    if (type == CFStringGetTypeID()) {
        return copyCFString((CFStringRef)value);
    }
    return NULL;
}

void dtAXRelease(CFTypeRef value) {
    if (value != NULL) {
        CFRelease(value);
    }
}

int dtAXIsNull(CFTypeRef value) {
    return value == NULL ? 1 : 0;
}

static void dtFillFrontmost(char **bundle, char **name, pid_t *pid) {
    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app == nil) {
            return;
        }
        *pid = app.processIdentifier;
        if (app.bundleIdentifier != nil) {
            *bundle = strdup(app.bundleIdentifier.UTF8String);
        }
        if (app.localizedName != nil) {
            *name = strdup(app.localizedName.UTF8String);
        }
    }
}

void dtFrontmost(char **bundle, char **name, pid_t *pid) {
    *bundle = NULL;
    *name = NULL;
    *pid = 0;
    // NSWorkspace's frontmost application is an AppKit query. Ask on the main
    // thread when an event loop is running, and directly otherwise (CLI use).
    if ([NSThread isMainThread] || NSApp == nil || ![NSApp isRunning]) {
        dtFillFrontmost(bundle, name, pid);
        return;
    }
    dispatch_sync(dispatch_get_main_queue(), ^{
        dtFillFrontmost(bundle, name, pid);
    });
}

int dtAppRunning(const char *bundleID, const char *name) {
    @autoreleasepool {
        NSString *wantedBundle = bundleID != NULL ? [NSString stringWithUTF8String:bundleID] : nil;
        NSString *wantedName = name != NULL ? [NSString stringWithUTF8String:name] : nil;
        for (NSRunningApplication *app in [NSWorkspace sharedWorkspace].runningApplications) {
            if (wantedBundle != nil && [app.bundleIdentifier isEqualToString:wantedBundle]) {
                return 1;
            }
            if (wantedName != nil && [app.localizedName isEqualToString:wantedName]) {
                return 1;
            }
        }
    }
    return 0;
}

// dtActivate brings the accessory (menu bar) app forward so a window opened
// from the status menu becomes key.
void dtActivate(void) {
    void (^block)(void) = ^{
        if (NSApp == nil) {
            return;
        }
        [NSApp activateIgnoringOtherApps:YES];
    };
    if ([NSThread isMainThread] || NSApp == nil || ![NSApp isRunning]) {
        block();
        return;
    }
    dispatch_sync(dispatch_get_main_queue(), block);
}
