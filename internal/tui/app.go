package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/jmcateer/switch-tester-5000/internal/config"
	"github.com/jmcateer/switch-tester-5000/internal/network"
	"github.com/rivo/tview"
)

const version = "0.1.0"

type App struct {
	tv      *tview.Application
	pages   *tview.Pages
	cfg     *config.Config
	cfgPath string

	role   string // "mirror" or "measure"
	mirror *network.Mirror

	// measure state
	mirrorIP       string
	mirrorPort     int
	listenPort     int
	switchName     string
	switchPorts    int
	measurePort    int
	lossThreshold  float64
	testDuration   int
	packetSize     int
	portResults    []network.TestResult
	testPortIndex  int // index into mirrorPorts slice
	mirrorPorts    []int
	testInProgress bool
}

func New(cfg *config.Config, cfgPath string) *App {
	a := &App{
		tv:      tview.NewApplication(),
		pages:   tview.NewPages(),
		cfg:     cfg,
		cfgPath: cfgPath,
	}
	a.loadFromConfig()
	return a
}

func (a *App) loadFromConfig() {
	a.mirrorIP = a.cfg.Network.MirrorIP
	a.mirrorPort = a.cfg.Network.MirrorPort
	a.listenPort = a.cfg.Network.ListenPort
	a.switchName = a.cfg.Switch.SwitchName
	a.switchPorts = a.cfg.Switch.PortCount
	a.measurePort = a.cfg.Switch.MeasurePort
	a.lossThreshold = a.cfg.Test.LossThresholdPercent
	a.testDuration = a.cfg.Test.TestDurationSeconds
	a.packetSize = a.cfg.Test.PacketSizeBytes
}

func (a *App) Run() error {
	a.tv.SetRoot(a.pages, true).EnableMouse(true)
	a.showWelcome()
	return a.tv.Run()
}

// switchTo replaces a named page and makes it the active page.
func (a *App) switchTo(name string, p tview.Primitive) {
	a.pages.RemovePage(name)
	a.pages.AddPage(name, p, true, true)
}

// ─── Welcome ─────────────────────────────────────────────────────────────────

func (a *App) showWelcome() {
	info := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf(
			"[yellow]Switch Tester 5000[white] v%s\n\n"+
				"This tool measures how much an unmanaged network\n"+
				"switch degrades performance on each port over time.\n\n"+
				"[green]Two computers are required:[white]\n"+
				"  • [cyan]Measuring Computer[white] — sends test traffic\n"+
				"  • [cyan]Mirroring Computer[white] — echoes traffic back\n\n"+
				"Both computers connect to the switch under test.\n"+
				"Start the mirroring computer [yellow]first[white].",
			version,
		))
	info.SetBorder(false)

	form := tview.NewForm().
		AddButton("Measuring Computer", func() {
			a.role = "measure"
			a.showMeasureSetup()
		}).
		AddButton("Mirroring Computer", func() {
			a.role = "mirror"
			a.showMirrorSetup()
		}).
		AddButton("Quit", func() {
			a.tv.Stop()
		})
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetBorder(false)
	form.SetBackgroundColor(tcell.ColorDefault)

	body := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(info, 12, 0, false).
		AddItem(form, 3, 0, true)
	body.SetBorder(true).SetTitle(" Switch Tester 5000 ").SetTitleAlign(tview.AlignCenter)
	body.SetBorderColor(tcell.ColorYellow)

	outer := centered(body, 64, 19)
	a.switchTo("welcome", outer)
	a.tv.SetFocus(form)
}

// ─── Mirror Setup ─────────────────────────────────────────────────────────────

func (a *App) showMirrorSetup() {
	listenPort := a.listenPort
	if listenPort == 0 {
		listenPort = 5000
	}
	portStr := strconv.Itoa(listenPort)

	form := tview.NewForm()
	form.AddInputField("Listen Port", portStr, 10, acceptDigits, func(v string) {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < 65536 {
			listenPort = n
		}
	})
	form.AddButton("Start Mirroring", func() {
		a.mirror = network.NewMirror(listenPort)
		if err := a.mirror.Start(); err != nil {
			a.showError("Failed to start mirror", err.Error(), func() { a.showMirrorSetup() })
			return
		}
		// save listen port back to config
		a.cfg.Network.MirrorPort = listenPort
		config.Save(a.cfgPath, a.cfg) //nolint:errcheck
		a.showMirrorRun(listenPort)
	})
	form.AddButton("Back", func() { a.showWelcome() })
	form.SetBorder(true).SetTitle(" Mirroring Computer Setup ").SetTitleAlign(tview.AlignCenter)
	form.SetBorderColor(tcell.ColorAqua)
	form.SetFieldBackgroundColor(tcell.ColorNavy)

	a.switchTo("mirror-setup", centered(form, 52, 10))
	a.tv.SetFocus(form)
}

// ─── Mirror Run ───────────────────────────────────────────────────────────────

func (a *App) showMirrorRun(port int) {
	status := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() { a.tv.Draw() })

	updateStatus := func() {
		s := a.mirror.Stats()
		elapsed := time.Since(s.StartTime)
		h := int(elapsed.Hours())
		m := int(elapsed.Minutes()) % 60
		sec := int(elapsed.Seconds()) % 60

		recvBps := float64(s.BytesReceived)
		if elapsed.Seconds() > 0 {
			recvBps = float64(s.BytesReceived) / elapsed.Seconds()
		}

		status.SetText(fmt.Sprintf(
			"[green]● ACTIVE[white]  Listening on UDP port [cyan]%d[white]\n\n"+
				"  Packets received : [yellow]%d[white]\n"+
				"  Packets echoed   : [yellow]%d[white]\n"+
				"  Bytes received   : [yellow]%s[white]\n"+
				"  Avg throughput   : [yellow]%s/s[white]\n"+
				"  Uptime           : [yellow]%02d:%02d:%02d[white]\n\n"+
				"[darkgray]This window must stay open during the test.[white]",
			port,
			s.PacketsReceived,
			s.PacketsEchoed,
			fmtBytes(s.BytesReceived),
			fmtBytes(uint64(recvBps)),
			h, m, sec,
		))
	}
	updateStatus()

	stopBtn := tview.NewButton("[Stop]").SetSelectedFunc(func() {
		a.mirror.Stop()
		a.mirror = nil
		a.showMirrorSetup()
	})

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(status, 0, 1, false).
		AddItem(stopBtn, 1, 0, true)
	layout.SetBorder(true).SetTitle(" Mirroring — Active ").SetTitleAlign(tview.AlignCenter)
	layout.SetBorderColor(tcell.ColorGreen)

	a.switchTo("mirror-run", centered(layout, 60, 16))
	a.tv.SetFocus(stopBtn)

	// periodic update ticker
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if a.mirror == nil {
				return
			}
			a.tv.QueueUpdateDraw(updateStatus)
		}
	}()
}

// ─── Measure Setup ────────────────────────────────────────────────────────────

func (a *App) showMeasureSetup() {
	mirrorIP := a.mirrorIP
	mirrorPort := a.mirrorPort
	listenPort := a.listenPort
	switchName := a.switchName
	switchPorts := a.switchPorts
	measurePort := a.measurePort
	lossThreshold := a.lossThreshold
	testDuration := a.testDuration
	packetSize := a.packetSize

	form := tview.NewForm()
	form.AddInputField("Mirror Computer IP", mirrorIP, 20, nil, func(v string) { mirrorIP = v })
	form.AddInputField("Mirror UDP Port", strconv.Itoa(mirrorPort), 8, acceptDigits, func(v string) {
		if n, err := strconv.Atoi(v); err == nil { mirrorPort = n }
	})
	form.AddInputField("Listen (reply) Port", strconv.Itoa(listenPort), 8, acceptDigits, func(v string) {
		if n, err := strconv.Atoi(v); err == nil { listenPort = n }
	})
	form.AddInputField("Switch Name", switchName, 20, nil, func(v string) { switchName = v })
	form.AddInputField("Switch Port Count", strconv.Itoa(switchPorts), 6, acceptDigits, func(v string) {
		if n, err := strconv.Atoi(v); err == nil && n > 1 { switchPorts = n }
	})
	form.AddInputField("Measuring Computer Port#", strconv.Itoa(measurePort), 6, acceptDigits, func(v string) {
		if n, err := strconv.Atoi(v); err == nil && n > 0 { measurePort = n }
	})
	form.AddInputField("Loss Threshold %", fmt.Sprintf("%.0f", lossThreshold), 6, acceptFloatDigits, func(v string) {
		if f, err := strconv.ParseFloat(v, 64); err == nil { lossThreshold = f }
	})
	form.AddInputField("Test Duration (sec)", strconv.Itoa(testDuration), 6, acceptDigits, func(v string) {
		if n, err := strconv.Atoi(v); err == nil && n > 0 { testDuration = n }
	})
	form.AddInputField("Packet Size (bytes)", strconv.Itoa(packetSize), 8, acceptDigits, func(v string) {
		if n, err := strconv.Atoi(v); err == nil && n >= 20 { packetSize = n }
	})
	form.AddButton("Save & Continue", func() {
		// validate
		if mirrorIP == "" {
			a.showError("Validation", "Mirror Computer IP is required.", func() { a.showMeasureSetup() })
			return
		}
		if measurePort < 1 || measurePort > switchPorts {
			a.showError("Validation", fmt.Sprintf("Measuring port must be between 1 and %d.", switchPorts), func() { a.showMeasureSetup() })
			return
		}

		// persist
		a.mirrorIP = mirrorIP
		a.mirrorPort = mirrorPort
		a.listenPort = listenPort
		a.switchName = switchName
		a.switchPorts = switchPorts
		a.measurePort = measurePort
		a.lossThreshold = lossThreshold
		a.testDuration = testDuration
		a.packetSize = packetSize

		a.cfg.Network.MirrorIP = mirrorIP
		a.cfg.Network.MirrorPort = mirrorPort
		a.cfg.Network.ListenPort = listenPort
		a.cfg.Switch.SwitchName = switchName
		a.cfg.Switch.PortCount = switchPorts
		a.cfg.Switch.MeasurePort = measurePort
		a.cfg.Test.LossThresholdPercent = lossThreshold
		a.cfg.Test.TestDurationSeconds = testDuration
		a.cfg.Test.PacketSizeBytes = packetSize
		config.Save(a.cfgPath, a.cfg) //nolint:errcheck

		// build list of ports to test (all except measurePort)
		a.mirrorPorts = nil
		for p := 1; p <= switchPorts; p++ {
			if p != measurePort {
				a.mirrorPorts = append(a.mirrorPorts, p)
			}
		}
		a.portResults = nil
		a.testPortIndex = 0
		a.showTestGuide()
	})
	form.AddButton("Back", func() { a.showWelcome() })

	form.SetBorder(true).SetTitle(" Measuring Computer Setup ").SetTitleAlign(tview.AlignCenter)
	form.SetBorderColor(tcell.ColorAqua)
	form.SetFieldBackgroundColor(tcell.ColorNavy)

	a.switchTo("measure-setup", centered(form, 60, 30))
	a.tv.SetFocus(form)
}

// ─── Test Guide ───────────────────────────────────────────────────────────────

func (a *App) showTestGuide() {
	if a.testPortIndex >= len(a.mirrorPorts) {
		a.showResults()
		return
	}

	currentMirrorPort := a.mirrorPorts[a.testPortIndex]
	total := len(a.mirrorPorts)
	done := a.testPortIndex

	var instruction string
	if a.testPortIndex == 0 {
		instruction = fmt.Sprintf(
			"[white]Initial Cable Setup:[white]\n\n"+
				"  1. Connect the [cyan]measuring computer[white] to\n"+
				"     [yellow]%s port %d[white]\n\n"+
				"  2. Connect the [cyan]mirroring computer[white] to\n"+
				"     [yellow]%s port %d[white]\n\n"+
				"  3. Ensure both computers have network\n"+
				"     connectivity through the switch\n\n"+
				"Press [green][Begin Test][white] when cables are connected.",
			a.switchName, a.measurePort,
			a.switchName, currentMirrorPort,
		)
	} else {
		prevPort := a.mirrorPorts[a.testPortIndex-1]
		instruction = fmt.Sprintf(
			"[white]Move Cable:[white]\n\n"+
				"  Move the [cyan]mirroring computer[white] cable\n"+
				"  from [yellow]%s port %d[white]\n"+
				"  to   [yellow]%s port %d[white]\n\n"+
				"  (Measuring computer stays on port %d)\n\n"+
				"Press [green][Begin Test][white] when cable is moved.",
			a.switchName, prevPort,
			a.switchName, currentMirrorPort,
			a.measurePort,
		)
	}

	// build past results summary
	var pastBuf strings.Builder
	if len(a.portResults) > 0 {
		pastBuf.WriteString("[white]Completed ports:\n")
		for _, r := range a.portResults {
			icon := "[green]PASS[white]"
			if r.MaxKbps == 0 {
				icon = "[red]FAIL[white]"
			}
			pastBuf.WriteString(fmt.Sprintf("  Port %-3d %s max %s\n", r.SwitchPort, icon, fmtKbps(r.MaxKbps)))
		}
	}

	instrView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(instruction)
	instrView.SetBorder(true).
		SetTitle(fmt.Sprintf(" Port %d of %d — Switch Port %d ", done+1, total, currentMirrorPort)).
		SetTitleAlign(tview.AlignCenter).
		SetBorderColor(tcell.ColorYellow)

	progressView := tview.NewTextView().
		SetDynamicColors(true)
	if pastBuf.Len() > 0 {
		progressView.SetText(pastBuf.String())
	}

	var beginBtn *tview.Button
	beginBtn = tview.NewButton(" Begin Test ").
		SetSelectedFunc(func() {
			a.runTest(currentMirrorPort, instrView, progressView, beginBtn)
		})
	beginBtn.SetBackgroundColor(tcell.ColorGreen)
	beginBtn.SetLabelColor(tcell.ColorBlack)

	backBtn := tview.NewButton(" Back to Setup ").
		SetSelectedFunc(func() { a.showMeasureSetup() })

	buttons := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(beginBtn, 14, 0, true).
		AddItem(tview.NewBox(), 2, 0, false).
		AddItem(backBtn, 16, 0, false).
		AddItem(nil, 0, 1, false)

	body := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(instrView, 12, 0, false).
		AddItem(progressView, 0, 1, false).
		AddItem(buttons, 1, 0, true)
	body.SetBorder(true).
		SetTitle(fmt.Sprintf(" Testing %s — %d/%d ports completed ", a.switchName, done, total)).
		SetTitleAlign(tview.AlignCenter).
		SetBorderColor(tcell.ColorAqua)

	a.switchTo("test-guide", centered(body, 70, 34))
	a.tv.SetFocus(beginBtn)
}

// ─── Run Test ─────────────────────────────────────────────────────────────────

func (a *App) runTest(switchPort int, instrView, progressView *tview.TextView, beginBtn *tview.Button) {
	if a.testInProgress {
		return
	}
	a.testInProgress = true

	// disable begin button visually
	a.tv.QueueUpdateDraw(func() {
		beginBtn.SetBackgroundColor(tcell.ColorGray)
		beginBtn.SetLabel(" Testing... ")
		instrView.SetText(fmt.Sprintf(
			"[yellow]Testing port %d...[white]\n\nPlease wait. Do not move cables.\n\nThis will take approximately %d seconds per bandwidth level.",
			switchPort,
			a.testDuration,
		))
	})

	sender := network.NewSender(a.mirrorIP, a.mirrorPort, a.listenPort, a.packetSize)

	go func() {
		// build bandwidth ladder: 500K, 1M, 2M, 4M, ... until failure
		startKbps := a.cfg.Test.StartBandwidthKbps
		if startKbps <= 0 {
			startKbps = 500
		}

		var results []network.BandwidthResult
		maxKbps := 0
		var progressLines []string

		addProgress := func(lines []string) {
			a.tv.QueueUpdateDraw(func() {
				progressView.SetText(strings.Join(lines, "\n"))
			})
		}

		// optional: ping first to verify connectivity
		a.tv.QueueUpdateDraw(func() {
			progressView.SetText("[yellow]Checking connectivity to mirror...[white]")
		})
		if err := sender.Ping(); err != nil {
			a.tv.QueueUpdateDraw(func() {
				a.testInProgress = false
				a.showError(
					fmt.Sprintf("Cannot reach mirror at %s:%d", a.mirrorIP, a.mirrorPort),
					err.Error(),
					func() {
						beginBtn.SetBackgroundColor(tcell.ColorGreen)
						beginBtn.SetLabel(" Begin Test ")
					},
				)
			})
			return
		}

		for kbps := startKbps; ; kbps *= 2 {
			label := fmtKbps(kbps)
			progressLines = append(progressLines, fmt.Sprintf("[white]%-8s [yellow]testing...[white]", label))
			addProgress(progressLines)

			result, err := sender.RunBandwidthLevel(
				kbps, a.testDuration, a.lossThreshold,
				func(u network.ProgressUpdate) {
					pct := 0.0
					if u.Sent > 0 {
						pct = float64(u.Received) / float64(u.Sent) * 100.0
					}
					last := len(progressLines) - 1
					progressLines[last] = fmt.Sprintf(
						"[white]%-8s [cyan]%s[white] %3.0f%% recv",
						label, progressBar(pct, 20), pct,
					)
					addProgress(progressLines)
				},
			)

			if err != nil {
				last := len(progressLines) - 1
				progressLines[last] = fmt.Sprintf("[white]%-8s [red]ERROR: %v[white]", label, err)
				addProgress(progressLines)
				break
			}

			results = append(results, result)

			pct := 100.0 - result.LossPercent
			last := len(progressLines) - 1
			if result.Passed {
				maxKbps = kbps
				progressLines[last] = fmt.Sprintf(
					"[white]%-8s [green]%s PASS[white] %.1f%% recv (%d/%d pkts)",
					label, progressBar(pct, 20), pct, result.Received, result.Sent,
				)
			} else {
				progressLines[last] = fmt.Sprintf(
					"[white]%-8s [red]%s FAIL[white] %.1f%% recv (%d/%d pkts)",
					label, progressBar(pct, 20), pct, result.Received, result.Sent,
				)
				addProgress(progressLines)
				break
			}
			addProgress(progressLines)

			// stop after 1Gbps even if passing
			if kbps >= 1024*1024 {
				break
			}
		}

		a.portResults = append(a.portResults, network.TestResult{
			SwitchPort: switchPort,
			Results:    results,
			MaxKbps:    maxKbps,
		})

		a.tv.QueueUpdateDraw(func() {
			a.testInProgress = false
			a.testPortIndex++
			statusLabel := "No throughput at minimum bandwidth"
			if maxKbps > 0 {
				statusLabel = fmt.Sprintf("Max passing bandwidth: %s", fmtKbps(maxKbps))
			}
			progressLines = append(progressLines, "", fmt.Sprintf("[green]Done — %s[white]", statusLabel))
			progressView.SetText(strings.Join(progressLines, "\n"))
			beginBtn.SetBackgroundColor(tcell.ColorYellow)
			if a.testPortIndex >= len(a.mirrorPorts) {
				beginBtn.SetLabel(" View Results ")
				beginBtn.SetSelectedFunc(func() { a.showResults() })
			} else {
				nextPort := a.mirrorPorts[a.testPortIndex]
				beginBtn.SetLabel(fmt.Sprintf(" Move to Port %d ", nextPort))
				beginBtn.SetSelectedFunc(func() { a.showTestGuide() })
			}
		})
	}()
}

// ─── Results ─────────────────────────────────────────────────────────────────

func (a *App) showResults() {
	if len(a.portResults) == 0 {
		a.showError("No Results", "No ports were tested.", func() { a.showTestGuide() })
		return
	}

	// find all bandwidth levels tested
	bwSet := map[int]struct{}{}
	for _, r := range a.portResults {
		for _, bw := range r.Results {
			bwSet[bw.BandwidthKbps] = struct{}{}
		}
	}
	bwLevels := sortedKeys(bwSet)

	table := tview.NewTable().SetBorders(true).SetFixed(1, 1)

	// header row
	table.SetCell(0, 0, hdr("Port"))
	for col, kbps := range bwLevels {
		table.SetCell(0, col+1, hdr(fmtKbps(kbps)))
	}
	table.SetCell(0, len(bwLevels)+1, hdr("Max BW"))
	table.SetCell(0, len(bwLevels)+2, hdr("Status"))

	// data rows
	for row, tr := range a.portResults {
		r := row + 1
		table.SetCell(r, 0, cell(fmt.Sprintf("%d", tr.SwitchPort)))
		resultMap := map[int]network.BandwidthResult{}
		for _, br := range tr.Results {
			resultMap[br.BandwidthKbps] = br
		}
		for col, kbps := range bwLevels {
			br, tested := resultMap[kbps]
			var c *tview.TableCell
			if !tested {
				c = tview.NewTableCell(" — ").SetAlign(tview.AlignCenter).SetTextColor(tcell.ColorGray)
			} else if br.Passed {
				c = tview.NewTableCell(" ✓ ").SetAlign(tview.AlignCenter).SetTextColor(tcell.ColorGreen)
			} else {
				c = tview.NewTableCell(" ✗ ").SetAlign(tview.AlignCenter).SetTextColor(tcell.ColorRed)
			}
			table.SetCell(r, col+1, c)
		}
		table.SetCell(r, len(bwLevels)+1, cell(fmtKbps(tr.MaxKbps)))
		status, color := portStatus(tr, a.lossThreshold)
		table.SetCell(r, len(bwLevels)+2, tview.NewTableCell(" "+status+" ").SetTextColor(color))
	}

	summary := a.buildSummary()
	summaryView := tview.NewTextView().SetDynamicColors(true).SetText(summary)
	summaryView.SetBorder(true).SetTitle(" Summary ").SetTitleAlign(tview.AlignCenter)

	doneBtn := tview.NewButton(" Done / Quit ").SetSelectedFunc(func() { a.tv.Stop() })
	againBtn := tview.NewButton(" Test Again ").SetSelectedFunc(func() {
		a.portResults = nil
		a.testPortIndex = 0
		a.showMeasureSetup()
	})

	buttons := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(againBtn, 14, 0, false).
		AddItem(tview.NewBox(), 2, 0, false).
		AddItem(doneBtn, 14, 0, true).
		AddItem(nil, 0, 1, false)

	body := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(table, 0, 2, true).
		AddItem(summaryView, 0, 1, false).
		AddItem(buttons, 1, 0, false)
	body.SetBorder(true).
		SetTitle(fmt.Sprintf(" Results — %s (%d ports tested) ", a.switchName, len(a.portResults))).
		SetTitleAlign(tview.AlignCenter).
		SetBorderColor(tcell.ColorYellow)

	a.switchTo("results", body)
	a.tv.SetFocus(doneBtn)
}

func (a *App) buildSummary() string {
	if len(a.portResults) == 0 {
		return "No data."
	}
	var good, degraded, bad int
	var maxes []int
	for _, r := range a.portResults {
		maxes = append(maxes, r.MaxKbps)
		switch portStatusCategory(r) {
		case "Good":
			good++
		case "Degraded":
			degraded++
		default:
			bad++
		}
	}

	avg := 0
	for _, m := range maxes {
		avg += m
	}
	avg /= len(maxes)

	lines := []string{
		fmt.Sprintf("[white]Ports tested: [cyan]%d[white]   "+
			"[green]Good: %d[white]   [yellow]Degraded: %d[white]   [red]Failed: %d[white]",
			len(a.portResults), good, degraded, bad),
		fmt.Sprintf("[white]Average max bandwidth: [cyan]%s[white]   "+
			"Loss threshold used: [cyan]%.0f%%[white]",
			fmtKbps(avg), a.lossThreshold),
	}
	if bad > 0 {
		lines = append(lines, "[red]Some ports failed at minimum bandwidth — hardware may be severely degraded.[white]")
	} else if degraded > 0 {
		lines = append(lines, "[yellow]Some ports show degraded performance — consider replacing the switch.[white]")
	} else {
		lines = append(lines, "[green]All tested ports passed — switch hardware appears healthy.[white]")
	}
	return strings.Join(lines, "\n")
}

// ─── Error Modal ──────────────────────────────────────────────────────────────

func (a *App) showError(title, msg string, onDismiss func()) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[red]%s[white]\n\n%s", title, msg)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(_ int, _ string) {
			a.pages.RemovePage("error")
			if onDismiss != nil {
				onDismiss()
			}
		})
	a.pages.AddPage("error", modal, true, true)
	a.tv.SetFocus(modal)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func centered(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(tview.NewBox(), 0, 1, false).
			AddItem(p, height, 0, true).
			AddItem(tview.NewBox(), 0, 1, false),
			width, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)
}

func acceptDigits(s string, r rune) bool {
	return r >= '0' && r <= '9'
}

func acceptFloatDigits(s string, r rune) bool {
	return (r >= '0' && r <= '9') || (r == '.' && !strings.Contains(s, "."))
}

func fmtBytes(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func fmtKbps(kbps int) string {
	if kbps == 0 {
		return "0K"
	}
	if kbps < 1000 {
		return fmt.Sprintf("%dK", kbps)
	}
	mbps := kbps / 1000
	if mbps < 1000 {
		return fmt.Sprintf("%dM", mbps)
	}
	return fmt.Sprintf("%dG", mbps/1000)
}

func progressBar(pct float64, width int) string {
	filled := int(math.Round(pct / 100.0 * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func hdr(s string) *tview.TableCell {
	return tview.NewTableCell(" " + s + " ").
		SetTextColor(tcell.ColorYellow).
		SetAlign(tview.AlignCenter).
		SetAttributes(tcell.AttrBold)
}

func cell(s string) *tview.TableCell {
	return tview.NewTableCell(" " + s + " ").SetAlign(tview.AlignCenter)
}

func portStatus(tr network.TestResult, _ float64) (string, tcell.Color) {
	cat := portStatusCategory(tr)
	switch cat {
	case "Good":
		return "Good", tcell.ColorGreen
	case "Degraded":
		return "Degraded", tcell.ColorYellow
	default:
		return "Failed", tcell.ColorRed
	}
}

func portStatusCategory(tr network.TestResult) string {
	if tr.MaxKbps == 0 {
		return "Failed"
	}
	// "Good" = passes at >= 10Mbps
	if tr.MaxKbps >= 10_000 {
		return "Good"
	}
	return "Degraded"
}

func sortedKeys(m map[int]struct{}) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// insertion sort (small slice)
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
