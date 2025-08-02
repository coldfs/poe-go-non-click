# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Windows desktop application written in Go that prevents accidental clicks during crafting in Path of Exile. The application monitors a specific pixel on screen and automatically moves the mouse when the target color is detected.

## Build Commands

### Prerequisites
- Go 1.21+ 
- TDM-GCC or MinGW-w64 (required for CGO)
- Windows (x64)

### Environment Setup
```bash
export PATH="/c/work/tdm-gcc/bin:/c/Program Files/Go/bin:$PATH"
export CGO_ENABLED=1
```

### Development Build (with console)
```bash
export PATH="/c/work/tdm-gcc/bin:/c/Program Files/Go/bin:$PATH"
export CGO_ENABLED=1
go build -v -o poe-go-non-click-fyne.exe main.go
```

### Production Build (without console)
```bash
export PATH="/c/work/tdm-gcc/bin:/c/Program Files/Go/bin:$PATH"
export CGO_ENABLED=1
go build -v -ldflags="-H windowsgui" -o poe-go-non-click-fyne.exe main.go
```

### Dependencies
```bash
go mod tidy
go mod download
```

## Architecture

### Single File Structure
- **main.go** - Complete application in one file (~428 lines)
- **FyneApp struct** - Main application state and GUI components
- **Windows API integration** - Direct syscalls to user32.dll and gdi32.dll

### Key Components
1. **GUI (Fyne v2)** - Cross-platform UI framework
2. **Pixel monitoring** - Real-time screen color detection using Windows GDI
3. **Thread safety** - All GUI updates use `fyne.Do()` for proper thread handling
4. **Global hotkeys** - Numpad 4/5 for quick control without focus

### Core Dependencies
- `fyne.io/fyne/v2` - GUI framework
- `github.com/go-vgo/robotgo` - System automation (indirect via imports)
- `github.com/kbinani/screenshot` - Screen capture capabilities
- Windows API calls via syscall package

### Application Flow
1. User selects pixel coordinates and target color
2. Monitoring loop checks pixel color every 10ms
3. On color match (2 consecutive matches), triggers action:
   - Plays system beep
   - Moves mouse 300 pixels up
   - Stops monitoring

### Thread Safety Notes
- Background goroutines use `fyne.Do()` for GUI updates
- Mutex protection for shared state (`isMonitoring`, counters)
- Separate goroutines for monitoring, statistics, and hotkey detection

## Development Notes

### CGO Requirement
This application requires CGO enabled due to Windows API calls and Fyne's native dependencies. Ensure TDM-GCC is properly installed and in PATH.

### Hotkey System
- Uses `GetAsyncKeyState` Windows API for global hotkey detection
- Runs in separate goroutine with proper key release detection
- No external hotkey libraries required

### Color Matching
- RGB tolerance of ±5 per channel for flexibility
- Requires 2 consecutive matches to prevent false positives
- 10ms check interval for responsive detection