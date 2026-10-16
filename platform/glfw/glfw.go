//go:build !js

// Package glfw is the slice of GLFW the engine uses: go-gl/glfw on the desktop,
// a canvas on js (glfw_js.go).
package glfw

import glfw "github.com/go-gl/glfw/v3.3/glfw"

type Action = glfw.Action
type Cursor = glfw.Cursor
type Key = glfw.Key
type ModifierKey = glfw.ModifierKey
type Monitor = glfw.Monitor
type MouseButton = glfw.MouseButton
type VidMode = glfw.VidMode
type Window = glfw.Window

var CreateCursor = glfw.CreateCursor
var CreateWindow = glfw.CreateWindow
var DetachCurrentContext = glfw.DetachCurrentContext
var GetPrimaryMonitor = glfw.GetPrimaryMonitor
var GetTime = glfw.GetTime
var Init = glfw.Init
var PollEvents = glfw.PollEvents
var SwapInterval = glfw.SwapInterval
var Terminate = glfw.Terminate
var WindowHint = glfw.WindowHint

const (
	CocoaRetinaFramebuffer  = glfw.CocoaRetinaFramebuffer
	ContextVersionMajor     = glfw.ContextVersionMajor
	ContextVersionMinor     = glfw.ContextVersionMinor
	CursorDisabled          = glfw.CursorDisabled
	CursorMode              = glfw.CursorMode
	CursorNormal            = glfw.CursorNormal
	Decorated               = glfw.Decorated
	DontCare                = glfw.DontCare
	DoubleBuffer            = glfw.DoubleBuffer
	False                   = glfw.False
	Key0                    = glfw.Key0
	Key1                    = glfw.Key1
	Key4                    = glfw.Key4
	Key7                    = glfw.Key7
	Key9                    = glfw.Key9
	KeyA                    = glfw.KeyA
	KeyApostrophe           = glfw.KeyApostrophe
	KeyB                    = glfw.KeyB
	KeyBackslash            = glfw.KeyBackslash
	KeyBackspace            = glfw.KeyBackspace
	KeyC                    = glfw.KeyC
	KeyCapsLock             = glfw.KeyCapsLock
	KeyComma                = glfw.KeyComma
	KeyD                    = glfw.KeyD
	KeyDelete               = glfw.KeyDelete
	KeyDown                 = glfw.KeyDown
	KeyE                    = glfw.KeyE
	KeyEnd                  = glfw.KeyEnd
	KeyEnter                = glfw.KeyEnter
	KeyEqual                = glfw.KeyEqual
	KeyEscape               = glfw.KeyEscape
	KeyF                    = glfw.KeyF
	KeyF1                   = glfw.KeyF1
	KeyF10                  = glfw.KeyF10
	KeyF11                  = glfw.KeyF11
	KeyF12                  = glfw.KeyF12
	KeyF2                   = glfw.KeyF2
	KeyF3                   = glfw.KeyF3
	KeyF4                   = glfw.KeyF4
	KeyF5                   = glfw.KeyF5
	KeyF6                   = glfw.KeyF6
	KeyF7                   = glfw.KeyF7
	KeyF8                   = glfw.KeyF8
	KeyF9                   = glfw.KeyF9
	KeyG                    = glfw.KeyG
	KeyGraveAccent          = glfw.KeyGraveAccent
	KeyHome                 = glfw.KeyHome
	KeyI                    = glfw.KeyI
	KeyInsert               = glfw.KeyInsert
	KeyJ                    = glfw.KeyJ
	KeyK                    = glfw.KeyK
	KeyKP0                  = glfw.KeyKP0
	KeyKP3                  = glfw.KeyKP3
	KeyKP9                  = glfw.KeyKP9
	KeyKPEnter              = glfw.KeyKPEnter
	KeyL                    = glfw.KeyL
	KeyLeft                 = glfw.KeyLeft
	KeyLeftAlt              = glfw.KeyLeftAlt
	KeyLeftBracket          = glfw.KeyLeftBracket
	KeyLeftControl          = glfw.KeyLeftControl
	KeyLeftShift            = glfw.KeyLeftShift
	KeyLeftSuper            = glfw.KeyLeftSuper
	KeyM                    = glfw.KeyM
	KeyMinus                = glfw.KeyMinus
	KeyO                    = glfw.KeyO
	KeyP                    = glfw.KeyP
	KeyPageDown             = glfw.KeyPageDown
	KeyPageUp               = glfw.KeyPageUp
	KeyPeriod               = glfw.KeyPeriod
	KeyQ                    = glfw.KeyQ
	KeyR                    = glfw.KeyR
	KeyRight                = glfw.KeyRight
	KeyRightAlt             = glfw.KeyRightAlt
	KeyRightBracket         = glfw.KeyRightBracket
	KeyRightControl         = glfw.KeyRightControl
	KeyRightShift           = glfw.KeyRightShift
	KeyRightSuper           = glfw.KeyRightSuper
	KeyS                    = glfw.KeyS
	KeySemicolon            = glfw.KeySemicolon
	KeySlash                = glfw.KeySlash
	KeySpace                = glfw.KeySpace
	KeyT                    = glfw.KeyT
	KeyTab                  = glfw.KeyTab
	KeyUp                   = glfw.KeyUp
	KeyV                    = glfw.KeyV
	KeyW                    = glfw.KeyW
	KeyX                    = glfw.KeyX
	KeyY                    = glfw.KeyY
	KeyZ                    = glfw.KeyZ
	ModControl              = glfw.ModControl
	ModShift                = glfw.ModShift
	MouseButtonLeft         = glfw.MouseButtonLeft
	MouseButtonRight        = glfw.MouseButtonRight
	OpenGLCoreProfile       = glfw.OpenGLCoreProfile
	OpenGLDebugContext      = glfw.OpenGLDebugContext
	OpenGLForwardCompatible = glfw.OpenGLForwardCompatible
	OpenGLProfile           = glfw.OpenGLProfile
	Press                   = glfw.Press
	Release                 = glfw.Release
	Repeat                  = glfw.Repeat
	Resizable               = glfw.Resizable
	True                    = glfw.True
	Visible                 = glfw.Visible
)

// Added for voxellib.
const (
	KeyN              = glfw.KeyN
	MouseButtonMiddle = glfw.MouseButtonMiddle
	RawMouseMotion    = glfw.RawMouseMotion
	Samples           = glfw.Samples
	Focused           = glfw.Focused
	FocusOnShow       = glfw.FocusOnShow
)

func RawMouseMotionSupported() bool { return glfw.RawMouseMotionSupported() }
