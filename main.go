package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"image/color"
	"log"
	"net/http"
	"sync"
	"syscall"
	"time"
	"unsafe"
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

type App struct {
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
}

type Status struct {
	IsMonitoring    bool   `json:"isMonitoring"`
	SelectedX       int    `json:"selectedX"`
	SelectedY       int    `json:"selectedY"`
	CurrentColor    string `json:"currentColor"`
	TargetColor     string `json:"targetColor"`
	CheckCount      int64  `json:"checkCount"`
	AvgChecksPerSec float64 `json:"avgChecksPerSec"`
	Status          string `json:"status"`
}

var app = &App{
	requiredMatches: 2,
}

const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>PoE Go Non-Click Protection</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 600px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .section { margin: 20px 0; padding: 15px; border: 1px solid #ddd; border-radius: 5px; }
        .section h3 { margin-top: 0; color: #333; }
        button { padding: 10px 15px; margin: 5px; cursor: pointer; border: none; border-radius: 4px; }
        .btn-primary { background: #007bff; color: white; }
        .btn-success { background: #28a745; color: white; }
        .btn-danger { background: #dc3545; color: white; }
        .btn-secondary { background: #6c757d; color: white; }
        .btn:disabled { opacity: 0.5; cursor: not-allowed; }
        .color-box { display: inline-block; width: 20px; height: 20px; border: 1px solid #ccc; margin-right: 10px; vertical-align: middle; }
        .status { padding: 10px; border-radius: 4px; margin: 10px 0; }
        .status.active { background: #d4edda; color: #155724; border: 1px solid #c3e6cb; }
        .status.inactive { background: #f8d7da; color: #721c24; border: 1px solid #f5c6cb; }
        .stats { display: flex; justify-content: space-between; }
        .stat-item { text-align: center; flex: 1; }
    </style>
</head>
<body>
    <div class="container">
        <h1>PoE Go Non-Click Protection</h1>
        
        <div class="section">
            <h3>Управление</h3>
            <button id="startBtn" class="btn btn-success" onclick="startMonitoring()">Начать мониторинг</button>
            <button id="stopBtn" class="btn btn-danger" onclick="stopMonitoring()" disabled>Остановить мониторинг</button>
        </div>
        
        <div class="section">
            <h3>Выбор пикселя</h3>
            <p>1. Нажмите кнопку "Захват координат"</p>
            <p>2. Наведите курсор на нужный пиксель</p>
            <p>3. Нажмите клавишу Enter</p>
            <button id="selectBtn" class="btn btn-primary" onclick="startCapture()">Захват координат</button>
            <p id="coords">Координаты: не выбраны</p>
        </div>
        
        <div class="section">
            <h3>Цвета</h3>
            <div style="margin: 10px 0;">
                <span class="color-box" id="currentColorBox"></span>
                <span>Текущий цвет: <span id="currentColor">RGB(0,0,0)</span></span>
            </div>
            <div style="margin: 10px 0;">
                <span class="color-box" id="targetColorBox"></span>
                <span>Целевой цвет: <span id="targetColor">RGB(0,0,0)</span></span>
            </div>
            <button id="setTargetBtn" class="btn btn-secondary" onclick="setTargetColor()" disabled>Назначить цвет целевым</button>
        </div>
        
        <div class="section">
            <h3>Статистика</h3>
            <div class="stats">
                <div class="stat-item">
                    <div>Проверок выполнено</div>
                    <div id="checkCount">0</div>
                </div>
                <div class="stat-item">
                    <div>Проверок в секунду</div>
                    <div id="avgChecks">0.0</div>
                </div>
            </div>
        </div>
        
        <div id="statusDiv" class="status inactive">
            <span id="statusText">Готов к работе</span>
        </div>
    </div>

    <script>
        let capturing = false;
        
        function updateStatus() {
            fetch('/status')
                .then(response => response.json())
                .then(data => {
                    document.getElementById('currentColor').textContent = data.currentColor;
                    document.getElementById('targetColor').textContent = data.targetColor;
                    document.getElementById('checkCount').textContent = data.checkCount;
                    document.getElementById('avgChecks').textContent = data.avgChecksPerSec.toFixed(1);
                    document.getElementById('statusText').textContent = data.status;
                    
                    if (data.selectedX > 0 && data.selectedY > 0) {
                        document.getElementById('coords').textContent = 'Координаты: (' + data.selectedX + ', ' + data.selectedY + ')';
                        document.getElementById('setTargetBtn').disabled = false;
                    }
                    
                    document.getElementById('startBtn').disabled = data.isMonitoring;
                    document.getElementById('stopBtn').disabled = !data.isMonitoring;
                    
                    const statusDiv = document.getElementById('statusDiv');
                    statusDiv.className = 'status ' + (data.isMonitoring ? 'active' : 'inactive');
                    
                    // Update color boxes
                    const currentMatch = data.currentColor.match(/RGB\((\d+),(\d+),(\d+)\)/);
                    if (currentMatch) {
                        document.getElementById('currentColorBox').style.backgroundColor = 
                            'rgb(' + currentMatch[1] + ',' + currentMatch[2] + ',' + currentMatch[3] + ')';
                    }
                    
                    const targetMatch = data.targetColor.match(/RGB\((\d+),(\d+),(\d+)\)/);
                    if (targetMatch) {
                        document.getElementById('targetColorBox').style.backgroundColor = 
                            'rgb(' + targetMatch[1] + ',' + targetMatch[2] + ',' + targetMatch[3] + ')';
                    }
                });
        }
        
        function startMonitoring() {
            fetch('/start', {method: 'POST'});
        }
        
        function stopMonitoring() {
            fetch('/stop', {method: 'POST'});
        }
        
        function startCapture() {
            capturing = true;
            document.getElementById('selectBtn').textContent = 'Ожидание Enter...';
            document.getElementById('selectBtn').disabled = true;
            fetch('/capture', {method: 'POST'});
            
            // Check capture status
            const checkCapture = setInterval(() => {
                fetch('/capture-status')
                    .then(response => response.json())
                    .then(data => {
                        if (!data.capturing) {
                            clearInterval(checkCapture);
                            document.getElementById('selectBtn').textContent = 'Захват координат';
                            document.getElementById('selectBtn').disabled = false;
                            capturing = false;
                        }
                    });
            }, 500);
        }
        
        function setTargetColor() {
            fetch('/set-target', {method: 'POST'});
        }
        
        // Update status every 100ms
        setInterval(updateStatus, 100);
        updateStatus();
    </script>
</body>
</html>`

func main() {
	fmt.Println("PoE Go Non-Click Protection")
	fmt.Println("Сервер запущен на http://localhost:8080")
	fmt.Println("Откройте браузер и перейдите по ссылке выше")
	
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/start", startHandler)
	http.HandleFunc("/stop", stopHandler)
	http.HandleFunc("/capture", captureHandler)
	http.HandleFunc("/capture-status", captureStatusHandler)
	http.HandleFunc("/set-target", setTargetHandler)
	
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("home").Parse(htmlTemplate))
	tmpl.Execute(w, nil)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	app.mu.RLock()
	status := Status{
		IsMonitoring:    app.isMonitoring,
		SelectedX:       app.selectedX,
		SelectedY:       app.selectedY,
		CurrentColor:    fmt.Sprintf("RGB(%d,%d,%d)", app.currentColor.R, app.currentColor.G, app.currentColor.B),
		TargetColor:     fmt.Sprintf("RGB(%d,%d,%d)", app.targetColor.R, app.targetColor.G, app.targetColor.B),
		CheckCount:      app.checkCount,
		Status:          getStatusText(),
	}
	
	if app.isMonitoring && !app.startTime.IsZero() {
		elapsed := time.Since(app.startTime).Seconds()
		if elapsed > 0 {
			status.AvgChecksPerSec = float64(app.checkCount) / elapsed
		}
	}
	app.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

var capturing = false

func captureHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	
	capturing = true
	go func() {
		for capturing {
			// Check if Enter key is pressed (VK_RETURN = 0x0D)
			ret, _, _ := procGetAsyncKeyState.Call(0x0D)
			if ret&0x8000 != 0 {
				x, y := getCursorPos()
				app.mu.Lock()
				app.selectedX = int(x)
				app.selectedY = int(y)
				app.mu.Unlock()
				
				updateCurrentColor()
				capturing = false
				
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

func captureStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"capturing": capturing})
}

func setTargetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	
	app.mu.Lock()
	app.targetColor = app.currentColor
	app.mu.Unlock()
}

func startHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	
	app.mu.Lock()
	if app.selectedX == 0 && app.selectedY == 0 {
		app.mu.Unlock()
		return
	}
	
	app.isMonitoring = true
	app.checkCount = 0
	app.startTime = time.Now()
	app.matchCount = 0
	app.mu.Unlock()
	
	go monitoringLoop()
}

func stopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	
	app.mu.Lock()
	app.isMonitoring = false
	app.mu.Unlock()
}

func monitoringLoop() {
	for {
		app.mu.RLock()
		if !app.isMonitoring {
			app.mu.RUnlock()
			break
		}
		app.mu.RUnlock()
		
		updateCurrentColor()
		app.mu.Lock()
		app.checkCount++
		app.mu.Unlock()
		
		// Check if colors match
		if colorsMatch(app.currentColor, app.targetColor) {
			app.matchCount++
			if app.matchCount >= app.requiredMatches {
				triggerAction()
				break
			}
		} else {
			app.matchCount = 0
		}
		
		time.Sleep(10 * time.Millisecond)
	}
}

func updateCurrentColor() {
	app.mu.RLock()
	x, y := app.selectedX, app.selectedY
	app.mu.RUnlock()
	
	if x == 0 && y == 0 {
		return
	}
	
	c := getPixelColor(x, y)
	app.mu.Lock()
	app.currentColor = c
	app.mu.Unlock()
}

func colorsMatch(c1, c2 color.RGBA) bool {
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

func triggerAction() {
	// Play system beep
	procMessageBeep.Call(0xFFFFFFFF)
	
	// Move mouse 500 pixels up
	currentX, currentY := getCursorPos()
	setCursorPos(currentX, currentY-500)
	
	// Stop monitoring
	app.mu.Lock()
	app.isMonitoring = false
	app.mu.Unlock()
}

func getStatusText() string {
	if app.isMonitoring {
		return "Мониторинг активен"
	}
	if app.selectedX == 0 && app.selectedY == 0 {
		return "Выберите пиксель для мониторинга"
	}
	return "Готов к мониторингу"
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
	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)
	
	colorRef, _, _ := procGetPixel.Call(hdc, uintptr(x), uintptr(y))
	
	r := uint8(colorRef & 0xFF)
	g := uint8((colorRef >> 8) & 0xFF)
	b := uint8((colorRef >> 16) & 0xFF)
	
	return color.RGBA{R: r, G: g, B: b, A: 255}
}