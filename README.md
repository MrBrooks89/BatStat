
_Disclaimer: This project was made with the assitance of Gemini_

# BatStat 🦇📊  

A **terminal user interface (TUI) tool** for monitoring, analyzing, and managing network connections in real time.  

---

## Table of Contents  
- [Main Interface](#main-interface)  
- [Features](#features)  
  - [Process Management](#process-management)  
  - [Network Diagnostics](#network-diagnostics)  
  - [Export to CSV](#export-to-csv)  
  - [In-App Help Panel](#in-app-help-panel)  
- [Installation](#installation)  
- [Usage](#usage)  
- [Contributing](#contributing)  
- [License](#license)  

---

## Main Interface  

The main view provides a **two-pane layout**:  

- **Left Pane:** Live-updating list of connections  
- **Right Pane:** Detailed information for the selected connection  

This design makes it easy to monitor and analyze network activity efficiently.  

<img width="2512" height="1357" alt="image" src="https://github.com/user-attachments/assets/369f3280-c83d-4180-8bf0-3b1e3177b38d" />

---

## Features  

### 🔴 Live Monitoring  
- Auto-refreshes the connection list every few seconds  
- Real-time filtering (`/` to filter by process name, PID, status, or address)  
- Color-coded connection states (`ESTABLISHED`, `LISTEN`, `CLOSE_WAIT`, etc.)  

### 📑 Two-Pane Layout  
- View all connections and details simultaneously  
- Column sorting:  
  - Press `s` → cycle through sortable columns  
  - Press `S` → toggle ascending/descending order  

### ⚙️ Process Management  
- `k` → Gracefully kill process for selected connection  
- `K` → Force kill with `SIGKILL`  

### 🌐 Network Diagnostics  
- `p` → Ping remote address in a live modal overlay  

### 📂 Export to CSV  
- `e` → Default export visible connections to `BatStat_export.csv` or Custom choose the path and file name 

### ❓ In-App Help Panel  
- `h` → Toggle a detailed, colorful panel with all keybindings  

---

## Installation  

Requires **Go 1.25+**.  

Latest verison:
```bash
go install github.com/MrBrooks89/BatStat/cmd/BatStat@latest
```
Or Specific version:
```bash
go install github.com/MrBrooks89/BatStat/cmd/BatStat@v0.1.0
```

Make sure your `GOPATH/bin` is in your `$PATH`:  
👉 [Go Wiki: Setting GOPATH](https://go.dev/wiki/SettingGOPATH)  

---

## Usage  

Run BatStat:  
```bash
BatStat
```  

Press `h` at any time for a full list of keybindings.  

---

## Contributing  

PRs are welcome!  
- Open an issue for bugs or feature requests  
- Fork and submit a pull request  

---

## License  

MIT License — see [LICENSE](LICENSE).
