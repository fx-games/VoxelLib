//go:build wgpu && darwin

package render

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore
#import <Cocoa/Cocoa.h>
#import <QuartzCore/CAMetalLayer.h>

static void *vl_metal_layer(void *win) {
	NSWindow *w = (__bridge NSWindow *)win;
	NSView *v = [w contentView];
	CAMetalLayer *l = [CAMetalLayer layer];
	l.contentsScale = w.backingScaleFactor;
	[v setLayer:l];
	[v setWantsLayer:YES];
	return (__bridge void *)l;
}

*/
import "C"

import "github.com/memmaker/voxellib/platform/glfw"

// metalLayer attaches a CAMetalLayer to the window's content view.
func metalLayer(win *glfw.Window) uintptr {
	return uintptr(C.vl_metal_layer(win.GetCocoaWindow()))
}
