package main

import (
	"fmt"
	"image/color"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	gdi32                = syscall.NewLazyDLL("gdi32.dll")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")
	procGetPixel         = gdi32.NewProc("GetPixel")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	procMessageBeep      = user32.NewProc("MessageBeep")
)

type POINT struct {
	X, Y int32
}

type FyneApp struct {
	window            fyne.Window
	isMonitoring      bool
	selectedX         int
	selectedY         int
	targetColor       color.RGBA
	currentColor      color.RGBA
	matchCount        int
	requiredMatches   int
	checkCount        int64
	startTime         time.Time
	mu                sync.RWMutex
	
	// GUI elements
	startBtn          *widget.Button
	stopBtn           *widget.Button
	selectPixelBtn    *widget.Button
	setTargetBtn      *widget.Button
	currentColorRect  *widget.Card
	targetColorRect   *widget.Card
	checkCountLabel   *widget.Label
	avgChecksLabel    *widget.Label
	statusLabel       *widget.Label
	coordsLabel       *widget.Label
}

func main() {
	a := app.New()
	a.SetIcon(nil)
	
	myApp := &FyneApp{
		window:          a.NewWindow("PoE Go Non-Click Protection"),
		requiredMatches: 2,
	}
	
	myApp.setupUI()
	myApp.window.ShowAndRun()
}

func (a *FyneApp) setupUI() {
	a.window.Resize(fyne.NewSize(500, 600))
	
	// Control block
	a.startBtn = widget.NewButton("Начать мониторинг", a.startMonitoring)
	a.stopBtn = widget.NewButton("Остановить мониторинг", a.stopMonitoring)
	a.stopBtn.Disable()
	
	controlBox := container.NewHBox(a.startBtn, a.stopBtn)
	controlGroup := widget.NewCard("Управление", "", controlBox)
	
	// Color block
	a.selectPixelBtn = widget.NewButton("Выбрать пиксель (Enter)", a.selectPixel)
	a.setTargetBtn = widget.NewButton("Назначить цвет целевым", a.setTargetColor)
	a.setTargetBtn.Disable()
	
	a.currentColorRect = widget.NewCard("Текущий цвет", "", widget.NewLabel("RGB(0,0,0)"))
	a.targetColorRect = widget.NewCard("Целевой цвет", "", widget.NewLabel("RGB(0,0,0)"))
	a.coordsLabel = widget.NewLabel("Координаты: не выбраны")
	
	colorButtonsBox := container.NewVBox(a.selectPixelBtn, a.setTargetBtn, a.coordsLabel)
	colorDisplayBox := container.NewHBox(a.currentColorRect, a.targetColorRect)
	colorBox := container.NewVBox(colorButtonsBox, colorDisplayBox)
	colorGroup := widget.NewCard("Цвета", "", colorBox)
	
	// Statistics block
	a.checkCountLabel = widget.NewLabel("Количество проверок: 0")
	a.avgChecksLabel = widget.NewLabel("Проверок в секунду: 0.0")
	
	statsBox := container.NewVBox(a.checkCountLabel, a.avgChecksLabel)
	statsGroup := widget.NewCard("Статистика", "", statsBox)
	
	// Status
	a.statusLabel = widget.NewLabel("Готов к работе")
	
	// Main layout
	content := container.NewVBox(
		controlGroup,
		colorGroup,
		statsGroup,
		a.statusLabel,
	)
	
	a.window.SetContent(content)
}

func (a *FyneApp) startMonitoring() {
	if a.selectedX == 0 && a.selectedY == 0 {
		a.statusLabel.SetText("Ошибка: Не выбран пиксель для мониторинга")
		return
	}
	
	a.mu.Lock()
	a.isMonitoring = true
	a.checkCount = 0
	a.startTime = time.Now()
	a.matchCount = 0
	a.mu.Unlock()
	
	a.startBtn.Disable()
	a.stopBtn.Enable()
	a.selectPixelBtn.Disable()
	a.setTargetBtn.Disable()
	
	a.statusLabel.SetText("Мониторинг активен")
	
	go a.monitoringLoop()
	go a.updateStatsLoop()
}

func (a *FyneApp) stopMonitoring() {
	a.mu.Lock()
	a.isMonitoring = false
	a.mu.Unlock()
	
	a.startBtn.Enable()
	a.stopBtn.Disable()
	a.selectPixelBtn.Enable()
	if a.selectedX != 0 && a.selectedY != 0 {
		a.setTargetBtn.Enable()
	}
	
	a.statusLabel.SetText("Мониторинг остановлен")
}

func (a *FyneApp) selectPixel() {
	a.statusLabel.SetText("Наведите курсор на нужный пиксель и нажмите Enter...")
	
	go func() {
		for {
			// Check if Enter key is pressed (VK_RETURN = 0x0D)
			ret, _, _ := procGetAsyncKeyState.Call(0x0D)
			if ret&0x8000 != 0 {
				x, y := getCursorPos()
				a.selectedX = int(x)
				a.selectedY = int(y)
				
				// Get current pixel color
				a.updateCurrentColor()
				
				a.setTargetBtn.Enable()
				a.coordsLabel.SetText(fmt.Sprintf("Координаты: (%d, %d)", x, y))
				a.statusLabel.SetText(fmt.Sprintf("Выбран пиксель: (%d, %d)", x, y))
				
				// Wait for key release
				for {
					ret, _, _ := procGetAsyncKeyState.Call(0x0D)
					if ret&0x8000 == 0 {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

func (a *FyneApp) setTargetColor() {
	a.targetColor = a.currentColor
	a.updateTargetColorDisplay()
	a.statusLabel.SetText("Целевой цвет установлен")
}

func (a *FyneApp) updateCurrentColor() {
	if a.selectedX == 0 && a.selectedY == 0 {
		return
	}
	
	c := getPixelColor(a.selectedX, a.selectedY)
	a.currentColor = c
	a.updateCurrentColorDisplay()
}

func (a *FyneApp) updateCurrentColorDisplay() {
	colorText := fmt.Sprintf("RGB(%d,%d,%d)", a.currentColor.R, a.currentColor.G, a.currentColor.B)
	a.currentColorRect.SetContent(widget.NewLabel(colorText))
}

func (a *FyneApp) updateTargetColorDisplay() {
	colorText := fmt.Sprintf("RGB(%d,%d,%d)", a.targetColor.R, a.targetColor.G, a.targetColor.B)
	a.targetColorRect.SetContent(widget.NewLabel(colorText))
}

func (a *FyneApp) monitoringLoop() {
	for {
		a.mu.RLock()
		if !a.isMonitoring {
			a.mu.RUnlock()
			break
		}
		a.mu.RUnlock()
		
		a.updateCurrentColor()
		a.mu.Lock()
		a.checkCount++
		a.mu.Unlock()
		
		// Check if colors match
		if a.colorsMatch(a.currentColor, a.targetColor) {
			a.matchCount++
			if a.matchCount >= a.requiredMatches {
				a.triggerAction()
				break
			}
		} else {
			a.matchCount = 0
		}
		
		time.Sleep(10 * time.Millisecond) // Check every 10ms
	}
}

func (a *FyneApp) updateStatsLoop() {
	for {
		a.mu.RLock()
		if !a.isMonitoring {
			a.mu.RUnlock()
			break
		}
		
		count := a.checkCount
		elapsed := time.Since(a.startTime).Seconds()
		a.mu.RUnlock()
		
		avgChecks := float64(count) / elapsed
		
		a.checkCountLabel.SetText(fmt.Sprintf("Количество проверок: %d", count))
		a.avgChecksLabel.SetText(fmt.Sprintf("Проверок в секунду: %.1f", avgChecks))
		
		time.Sleep(100 * time.Millisecond)
	}
}

func (a *FyneApp) colorsMatch(c1, c2 color.RGBA) bool {
	// Allow small tolerance for color matching
	tolerance := uint8(5)
	
	return abs(c1.R, c2.R) <= tolerance &&
		   abs(c1.G, c2.G) <= tolerance &&
		   abs(c1.B, c2.B) <= tolerance
}

func abs(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func (a *FyneApp) triggerAction() {
	// Play system beep
	procMessageBeep.Call(0xFFFFFFFF)
	
	// Move mouse 300 pixels up
	currentX, currentY := getCursorPos()
	setCursorPos(currentX, currentY-300)
	
	// Stop monitoring
	a.stopMonitoring()
	a.statusLabel.SetText("СРАБОТКА! Мышь перемещена, мониторинг остановлен")
}

// Windows API functions
func getCursorPos() (int32, int32) {
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt.X, pt.Y
}

func setCursorPos(x, y int32) {
	procSetCursorPos.Call(uintptr(x), uintptr(y))
}

func getPixelColor(x, y int) color.RGBA {
	hdc, _, _ := procGetDC.Call(0) // Get desktop DC
	defer procReleaseDC.Call(0, hdc)
	
	colorRef, _, _ := procGetPixel.Call(hdc, uintptr(x), uintptr(y))
	
	// Convert COLORREF to RGB
	r := uint8(colorRef & 0xFF)
	g := uint8((colorRef >> 8) & 0xFF)
	b := uint8((colorRef >> 16) & 0xFF)
	
	return color.RGBA{R: r, G: g, B: b, A: 255}
}