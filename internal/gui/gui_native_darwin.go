//go:build darwin

package gui

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

static NSWindow *tuxWindow = nil;
static NSMutableArray *tuxParents = nil;
static NSMutableDictionary *tuxInputs = nil;
static NSString *tuxAction = nil;
static NSString *tuxTarget = nil;

static NSColor *tuxColorFromHex(const char *value) {
	if (value == NULL || *value == '\0') {
		return nil;
	}

	NSString *hex = [NSString stringWithUTF8String:value];
	if ([hex hasPrefix:@"#"]) {
		hex = [hex substringFromIndex:1];
	}
	if ([hex length] != 6) {
		return nil;
	}

	unsigned int rgb = 0;
	[[NSScanner scannerWithString:hex] scanHexInt:&rgb];
	CGFloat red = ((rgb >> 16) & 0xFF) / 255.0;
	CGFloat green = ((rgb >> 8) & 0xFF) / 255.0;
	CGFloat blue = (rgb & 0xFF) / 255.0;
	return [NSColor colorWithCalibratedRed:red green:green blue:blue alpha:1.0];
}

static NSDictionary *tuxBuildResult(NSString *action, NSString *target) {
	NSMutableDictionary *values = [NSMutableDictionary dictionary];
	for (NSString *key in tuxInputs) {
		NSTextField *field = tuxInputs[key];
		values[key] = field.stringValue ?: @"";
	}
	return @{
		@"action": action ?: @"close",
		@"target": target ?: @"",
		@"values": values,
	};
}

@interface TuxButtonTarget : NSObject
@property(nonatomic, strong) NSString *target;
@end

@implementation TuxButtonTarget
- (void)pressed:(id)sender {
	tuxAction = @"click";
	tuxTarget = self.target ?: @"";
	[NSApp stopModal];
	[tuxWindow orderOut:nil];
}
@end

@interface TuxWindowDelegate : NSObject <NSWindowDelegate>
@end

@implementation TuxWindowDelegate
- (void)windowWillClose:(NSNotification *)notification {
	if (tuxAction == nil) {
		tuxAction = @"close";
	}
	if (tuxTarget == nil) {
		tuxTarget = @"";
	}
	[NSApp stopModal];
}
@end

static TuxWindowDelegate *tuxDelegate = nil;
static NSMutableArray *tuxButtonTargets = nil;

static void tuxInitWindow(const char *title, int x, int y, int width, int height, const char *background) {
	[NSApplication sharedApplication];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];

	tuxAction = nil;
	tuxTarget = nil;
	tuxInputs = [NSMutableDictionary dictionary];
	tuxParents = [NSMutableArray array];
	tuxButtonTargets = [NSMutableArray array];

	NSRect frame = NSMakeRect(x, y, width, height);
	tuxWindow = [[NSWindow alloc] initWithContentRect:frame
		styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable)
		backing:NSBackingStoreBuffered
		defer:NO];
	[tuxWindow setTitle:[NSString stringWithUTF8String:(title == NULL ? "" : title)]];
	[tuxWindow center];

	NSView *contentView = [tuxWindow contentView];
	[contentView setWantsLayer:YES];
	NSColor *windowColor = tuxColorFromHex(background);
	if (windowColor != nil) {
		contentView.layer.backgroundColor = windowColor.CGColor;
	}

	tuxDelegate = [TuxWindowDelegate new];
	[tuxWindow setDelegate:tuxDelegate];
	[tuxParents addObject:contentView];
}

static NSView *tuxParentAt(int handle) {
	return tuxParents[handle];
}

static NSRect tuxFrameForParent(int parent, int x, int y, int width, int height) {
	NSView *view = tuxParentAt(parent);
	CGFloat parentHeight = view.bounds.size.height;
	return NSMakeRect(x, parentHeight - y - height, width, height);
}

static int tuxAddPanel(int parent, const char *background, int x, int y, int width, int height) {
	NSView *panel = [[NSView alloc] initWithFrame:tuxFrameForParent(parent, x, y, width, height)];
	[panel setWantsLayer:YES];
	NSColor *bg = tuxColorFromHex(background);
	if (bg != nil) {
		panel.layer.backgroundColor = bg.CGColor;
	}
	[tuxParentAt(parent) addSubview:panel];
	[tuxParents addObject:panel];
	return (int)[tuxParents count] - 1;
}

static void tuxAddLabel(int parent, const char *text, int x, int y, int width, int height, const char *foreground, const char *background, int fontSize) {
	NSTextField *label = [NSTextField labelWithString:[NSString stringWithUTF8String:(text == NULL ? "" : text)]];
	[label setFrame:tuxFrameForParent(parent, x, y, width, height)];
	[label setEditable:NO];
	[label setBezeled:NO];
	[label setBordered:NO];
	[label setDrawsBackground:NO];
	if (fontSize > 0) {
		[label setFont:[NSFont systemFontOfSize:fontSize]];
	}
	NSColor *fg = tuxColorFromHex(foreground);
	if (fg != nil) {
		[label setTextColor:fg];
	}
	NSColor *bg = tuxColorFromHex(background);
	if (bg != nil) {
		[label setDrawsBackground:YES];
		[label setBackgroundColor:bg];
	}
	[tuxParentAt(parent) addSubview:label];
}

static void tuxAddButton(int parent, const char *target, const char *text, int x, int y, int width, int height, const char *foreground, const char *background, int fontSize) {
	NSButton *button = [[NSButton alloc] initWithFrame:tuxFrameForParent(parent, x, y, width, height)];
	[button setTitle:[NSString stringWithUTF8String:(text == NULL ? "" : text)]];
	[button setBezelStyle:NSBezelStyleRounded];
	if (fontSize > 0) {
		[button setFont:[NSFont systemFontOfSize:fontSize]];
	}
	NSColor *fg = tuxColorFromHex(foreground);
	if (fg != nil) {
		[button setContentTintColor:fg];
	}
	NSColor *bg = tuxColorFromHex(background);
	if (bg != nil) {
		[button setWantsLayer:YES];
		button.layer.backgroundColor = bg.CGColor;
		button.layer.cornerRadius = 6.0;
	}

	TuxButtonTarget *buttonTarget = [TuxButtonTarget new];
	buttonTarget.target = [NSString stringWithUTF8String:(target == NULL ? "" : target)];
	[button setTarget:buttonTarget];
	[button setAction:@selector(pressed:)];
	[tuxButtonTargets addObject:buttonTarget];
	[tuxParentAt(parent) addSubview:button];
}

static void tuxAddEntry(int parent, const char *entryID, const char *text, const char *placeholder, int x, int y, int width, int height, const char *foreground, const char *background, int fontSize) {
	NSTextField *field = [[NSTextField alloc] initWithFrame:tuxFrameForParent(parent, x, y, width, height)];
	[field setStringValue:[NSString stringWithUTF8String:(text == NULL ? "" : text)]];
	if (placeholder != NULL && *placeholder != '\0') {
		[field setPlaceholderString:[NSString stringWithUTF8String:placeholder]];
	}
	if (fontSize > 0) {
		[field setFont:[NSFont systemFontOfSize:fontSize]];
	}
	NSColor *fg = tuxColorFromHex(foreground);
	if (fg != nil) {
		[field setTextColor:fg];
	}
	NSColor *bg = tuxColorFromHex(background);
	if (bg != nil) {
		[field setBackgroundColor:bg];
	}
	NSString *key = [NSString stringWithUTF8String:(entryID == NULL ? "" : entryID)];
	tuxInputs[key] = field;
	[tuxParentAt(parent) addSubview:field];
}

static char *tuxRunWindow(void) {
	[tuxWindow makeKeyAndOrderFront:nil];
	[NSApp activateIgnoringOtherApps:YES];
	[NSApp runModalForWindow:tuxWindow];

	NSDictionary *result = tuxBuildResult(tuxAction, tuxTarget);
	NSData *json = [NSJSONSerialization dataWithJSONObject:result options:0 error:nil];
	NSString *jsonText = [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
	[tuxWindow close];
	tuxWindow = nil;
	return strdup([jsonText UTF8String]);
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

func showNativeDesktop(w *Window) (*Result, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	title := C.CString(w.title)
	background := C.CString(w.background)
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(background))

	C.tuxInitWindow(title, C.int(w.x), C.int(w.y), C.int(w.width), C.int(w.height), background)
	if err := renderDarwinWidgets(0, w.children); err != nil {
		return nil, err
	}

	raw := C.tuxRunWindow()
	defer C.free(unsafe.Pointer(raw))
	return decodeResult([]byte(C.GoString(raw)))
}

func renderDarwinWidgets(parent int, widgets []Widget) error {
	for i, widget := range widgets {
		base := widget.base()
		id := base.id
		if id == "" {
			id = fmt.Sprintf("%s_%d", widget.kind(), i)
		}

		switch value := widget.(type) {
		case *Label:
			if err := darwinAddLabel(parent, value.text, base); err != nil {
				return err
			}
		case *Button:
			if err := darwinAddButton(parent, id, value.text, base); err != nil {
				return err
			}
		case *TextInput:
			if err := darwinAddEntry(parent, id, value.text, value.placeholder, base); err != nil {
				return err
			}
		case *Panel:
			handle, err := darwinAddPanel(parent, base)
			if err != nil {
				return err
			}
			if err := renderDarwinWidgets(handle, value.children); err != nil {
				return err
			}
		}
	}
	return nil
}

func darwinAddPanel(parent int, base *baseWidget) (int, error) {
	background := C.CString(base.background)
	defer C.free(unsafe.Pointer(background))

	handle := C.tuxAddPanel(
		C.int(parent),
		background,
		C.int(base.x),
		C.int(base.y),
		C.int(base.width),
		C.int(base.height),
	)
	return int(handle), nil
}

func darwinAddLabel(parent int, text string, base *baseWidget) error {
	cText := C.CString(text)
	cForeground := C.CString(base.foreground)
	cBackground := C.CString(base.background)
	defer C.free(unsafe.Pointer(cText))
	defer C.free(unsafe.Pointer(cForeground))
	defer C.free(unsafe.Pointer(cBackground))

	C.tuxAddLabel(
		C.int(parent),
		cText,
		C.int(base.x),
		C.int(base.y),
		C.int(base.width),
		C.int(base.height),
		cForeground,
		cBackground,
		C.int(base.fontSize),
	)
	return nil
}

func darwinAddButton(parent int, id string, text string, base *baseWidget) error {
	cID := C.CString(id)
	cText := C.CString(text)
	cForeground := C.CString(base.foreground)
	cBackground := C.CString(base.background)
	defer C.free(unsafe.Pointer(cID))
	defer C.free(unsafe.Pointer(cText))
	defer C.free(unsafe.Pointer(cForeground))
	defer C.free(unsafe.Pointer(cBackground))

	C.tuxAddButton(
		C.int(parent),
		cID,
		cText,
		C.int(base.x),
		C.int(base.y),
		C.int(base.width),
		C.int(base.height),
		cForeground,
		cBackground,
		C.int(base.fontSize),
	)
	return nil
}

func darwinAddEntry(parent int, id string, text string, placeholder string, base *baseWidget) error {
	cID := C.CString(id)
	cText := C.CString(text)
	cPlaceholder := C.CString(placeholder)
	cForeground := C.CString(base.foreground)
	cBackground := C.CString(base.background)
	defer C.free(unsafe.Pointer(cID))
	defer C.free(unsafe.Pointer(cText))
	defer C.free(unsafe.Pointer(cPlaceholder))
	defer C.free(unsafe.Pointer(cForeground))
	defer C.free(unsafe.Pointer(cBackground))

	C.tuxAddEntry(
		C.int(parent),
		cID,
		cText,
		cPlaceholder,
		C.int(base.x),
		C.int(base.y),
		C.int(base.width),
		C.int(base.height),
		cForeground,
		cBackground,
		C.int(base.fontSize),
	)
	return nil
}
