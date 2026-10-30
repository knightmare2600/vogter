package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/poller"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

// PollerFmt gives the poller the formatting used for host facts.
var PollerFmt = poller.Fmt{Bytes: fmtBytes, Duration: fmtDuration}

func fmtBytes(v float64) string { return scaled(v, 1024, "B", "KiB", "MiB", "GiB", "TiB", "PiB") }

func fmtBits(v float64) string { return scaled(v, 1000, "bps", "Kbps", "Mbps", "Gbps", "Tbps") }

func scaled(v, step float64, units ...string) string {
	i := 0
	for math.Abs(v) >= step && i < len(units)-1 {
		v /= step
		i++
	}
	if i == 0 || v >= 100 {
		return fmt.Sprintf("%.0f %s", v, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

// fmtDuration is compact: 45s, 12m, 3h 5m, 4d 2h.
func fmtDuration(secs int64) string {
	s := max(secs, 0)
	d, h, m := s/86400, s%86400/3600, s%3600/60
	switch {
	case d == 0 && h == 0 && m == 0:
		return fmt.Sprintf("%ds", s)
	case d == 0 && h == 0:
		return fmt.Sprintf("%dm", m)
	case d == 0 && m == 0:
		return fmt.Sprintf("%dh", h)
	case d == 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h == 0:
		return fmt.Sprintf("%dd", d)
	}
	return fmt.Sprintf("%dd %dh", d, h)
}

func fmtAge(clock int64, now time.Time) string { return fmtDuration(now.Unix() - clock) }

// fmtClock shows the time for the last day, else the date too.
func fmtClock(ts int64, now time.Time) string {
	t := time.Unix(ts, 0)
	if now.Unix()-ts < 86400 {
		return t.Format("15:04:05")
	}
	return t.Format("Jan 02 15:04")
}

func fmtHHMM(ts float64) string { return time.Unix(int64(ts), 0).Format("15:04") }

func fmtPct(m model.Metric) string {
	switch {
	case !m.Set:
		return "-"
	case m.Value >= 99.95:
		return "100%"
	case m.Value >= 10:
		return fmt.Sprintf("%.0f%%", m.Value)
	}
	return fmt.Sprintf("%.1f%%", m.Value)
}

func fmtNumber(v float64) string {
	switch {
	case v == math.Trunc(v) && math.Abs(v) < 1e15:
		return fmt.Sprintf("%.0f", v)
	case math.Abs(v) >= 100:
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

// fmtMetric formats a value with f, or "-" when it is missing.
func fmtMetric(m model.Metric, f func(float64) string) string {
	if !m.Set {
		return "-"
	}
	return f(m.Value)
}

func fmtLoad(v float64) string { return fmt.Sprintf("%.2f", v) }

func fmtUptime(v float64) string { return fmtDuration(int64(v)) }

// fmtItemValue renders an item's last value the way the frontend would.
func fmtItemValue(it zabbix.Item, now time.Time) string {
	if it.LastClock == "0" {
		return "-"
	}
	if !it.Numeric() {
		v := strings.NewReplacer("\n", " ", "\r", " ").Replace(it.LastValue)
		if v == "" {
			return `""`
		}
		return v
	}
	v, err := strconv.ParseFloat(it.LastValue, 64)
	if err != nil {
		return it.LastValue
	}
	return fmtUnits(v, it.Units, now)
}

func fmtUnits(v float64, units string, now time.Time) string {
	switch units {
	case "B":
		return fmtBytes(v)
	case "Bps":
		return fmtBytes(v) + "/s"
	case "bps":
		return fmtBits(v)
	case "uptime", "s":
		return fmtDuration(int64(v))
	case "unixtime":
		return fmtClock(int64(v), now)
	case "%":
		return fmt.Sprintf("%.1f%%", v)
	case "":
		return fmtNumber(v)
	}
	return fmtNumber(v) + " " + units
}
