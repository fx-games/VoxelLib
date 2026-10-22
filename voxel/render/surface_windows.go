package render

import (
	"unsafe"

	"github.com/gogpu/wgpu"
	"github.com/memmaker/voxellib/platform/glfw"
	"golang.org/x/sys/windows"
)

// surfaceTarget returns the window's HWND for a DX12/Vulkan surface.
func surfaceTarget(win *glfw.Window) wgpu.SurfaceTargetUnsafe {
	var inst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &inst)
	return wgpu.SurfaceTargetFromWindowsHWND(uintptr(inst), uintptr(unsafe.Pointer(win.GetWin32Window())))
}
