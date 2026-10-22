package render

import (
	"unsafe"

	glfw "github.com/go-gl/glfw/v3.3/glfw"
	"github.com/gogpu/wgpu"
)

// surfaceTarget returns the window's X11 display and window for a Vulkan
// surface. ponytail: X11 only; add glfw's Wayland accessors for Wayland.
func surfaceTarget(win *glfw.Window) wgpu.SurfaceTargetUnsafe {
	return wgpu.SurfaceTargetFromXlibWindow(uintptr(unsafe.Pointer(glfw.GetX11Display())), uintptr(win.GetX11Window()))
}
