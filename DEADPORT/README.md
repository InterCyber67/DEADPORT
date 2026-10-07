# 💀 DEADPORT
### "The internet's worst IT department."

DEADPORT is a multiplayer, interactive terminal-based game accessible exclusively via SSH. Players connect to the "DEADPORT Mainframe" to solve Linux-based puzzles, crack ciphers, find hidden files, and uncover the mess left behind by a rogue employee named Bob.

![Golang](https://img.shields.io/badge/Made_with-Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![BubbleTea](https://img.shields.io/badge/UI-Bubble_Tea-FF00FF?style=for-the-badge)
![SQLite](https://img.shields.io/badge/Database-SQLite-003B57?style=for-the-badge&logo=sqlite&logoColor=white)

## ✨ Features
- **🎮 Playable via SSH:** No client installation required. Just `ssh -p 2222 localhost`.
- **🖥️ Beautiful TUI:** Built with Charm's `bubbletea` and `lipgloss` featuring a split-screen terminal layout with animated cursors and syntax highlighting.
- **🌐 Global Chat:** Real-time multiplayer pub/sub chat system built right into the terminal shell.
- **🛡️ Sandboxed Shell Environment:** Simulates a fully functional Linux filesystem and command execution (like `ls`, `cd`, `grep`, `base64`) natively in Go—no actual `os/exec` commands are run, ensuring maximum security.
- **🏆 XP & Ranking System:** Uses `modernc.org/sqlite` (cgo-free) to persistently track player progress, XP, and completed missions.
- **🧩 20 Data-Driven Missions:** Challenges ranging from basic filesystem navigation to complex cryptography (`rot13`, `caesar`, `hex`), loaded dynamically from JSON files.

## 🚀 How to Play (Local)

1. Clone this repository.
2. Build and run the server:
   ```bash
   go build -o deadport .
   ./deadport
   ```
3. Open a new terminal and connect:
   ```bash
   ssh -p 2222 localhost -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null
   ```

## 🛠️ Deployment (Hack Club Nest / Linux)
DEADPORT is designed to be easily deployed on Linux servers or containers. 

**Using Docker (Recommended):**
```bash
docker build -t deadport .
docker run -d -p 2222:2222 -v deadport_data:/app/data deadport
```

**Using Systemd:**
Copy the included `deadport.service` to `/etc/systemd/system/`, reload the daemon, and start the service to run it as a background process with strict filesystem sandboxing.

---
*"Remember: If you break production, it comes out of your paycheck." — HR*
