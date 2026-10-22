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

import (
	"github.com/gogpu/wgpu"
	"github.com/memmaker/voxellib/platform/glfw"
)

// surfaceTarget attaches a CAMetalLayer to the window's content view.
func surfaceTarget(win *glfw.Window) wgpu.SurfaceTargetUnsafe {
	return wgpu.SurfaceTargetFromMetalLayer(uintptr(C.vl_metal_layer(win.GetCocoaWindow())))
}
