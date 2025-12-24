package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ActivePeriod represents a contiguous period of activity
type ActivePeriod struct {
	Start    time.Time
	End      time.Time
	Duration time.Duration
}

func main() {
	since, before := beforeMonth()
	sincePtr := flag.String("since", since+" 00:00:00 "+timeZoneOffset(), "since(after) date")
	beforePtr := flag.String("before", before+" 23:59:59 "+timeZoneOffset(), "before date")
	authorPtr := flag.String("author", "", "author name")            // git option : --author="\(Adam\)\|\(Jon\)"
	durationPtr := flag.String("duration", "1h", "git log duration") // default "1h"
	debugPtr := flag.Bool("debug", false, "debug mode")
	periodsPtr := flag.Bool("periods", false, "show list of active periods")
	helpPtr := flag.Bool("help", false, "print help")
	allBranchesPtr := flag.Bool("all", false, "include all branches in git log")
	reflogPtr := flag.Bool("reflog", false, "walk reflog entries in git log")
	flag.Parse()
	if *helpPtr {
		flag.PrintDefaults()
		os.Exit(0)
	}
	//checkMultiname
	var author string
	if strings.Contains(*authorPtr, ",") {
		author += `\(`
		author += strings.Join(strings.Split(*authorPtr, ","), `\)\|\(`)
		author += `\)`
	} else {
		author = *authorPtr
	}
	gitArgs := []string{"--no-pager", "log"}
	if *reflogPtr {
		gitArgs = append(gitArgs, "--walk-reflogs")
	}
	if *allBranchesPtr {
		gitArgs = append(gitArgs, "--all")
	}
	gitArgs = append(gitArgs,
		"--date=iso-local",
		`--pretty=format:%ad|%cd|%an|%s`,
		fmt.Sprintf(`--author=%s`, author),
		fmt.Sprintf(`--since="%s"`, *sincePtr),
		fmt.Sprintf(`--before="%s"`, *beforePtr),
	)
	cmd := exec.Command("git", gitArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		// Check if the error is because git is not installed
		if ee, ok := err.(*exec.Error); ok && ee.Err == exec.ErrNotFound {
			fmt.Fprintln(os.Stderr, "Error: git is not installed or not found in PATH.")
			os.Exit(1)
		}
		// If stderr has output, print it
		if stderr.String() != "" {
			fmt.Fprintf(os.Stderr, stderr.String())
		} else {
			// Otherwise, print the error itself
			fmt.Fprintf(os.Stderr, "Error running git: %v\n", err)
		}
		os.Exit(1)
	}
	if stderr.String() != "" {
		fmt.Fprintf(os.Stderr, stderr.String())
		os.Exit(1)
	}
	total, err := time.ParseDuration("0h")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if stdout.String() == "" {
		fmt.Printf("From %q to %q : %s\n", *sincePtr, *beforePtr, total)
		os.Exit(0)
	}

	var beforeCommitTime time.Time
	var activePeriods []ActivePeriod
	var currentPeriod *ActivePeriod
	lines := strings.Split(stdout.String(), "\n")
	// Reverse the lines to process from oldest to newest
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	for n, l := range lines {
		// Expecting format: %ad|%cd|%an|%s
		parts := strings.SplitN(l, "|", 4)
		if len(parts) < 4 {
			continue
		}
		authorDateStr := parts[0]
		commitDateStr := parts[1]
		// Parse author date
		authorRFC, err := ISO8601ToRFC3339(findISO8601.FindString(authorDateStr))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}
		authorTime, err := time.Parse(time.RFC3339, authorRFC)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}
		elapsed := authorTime.Sub(beforeCommitTime)
		// Fallback: if elapsed is negative, use commit date instead
		usedFallback := false
		if elapsed < 0 {
			usedFallback = true
			// Parse commit date
			commitRFC, err := ISO8601ToRFC3339(findISO8601.FindString(commitDateStr))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				continue
			}
			commitTime, err := time.Parse(time.RFC3339, commitRFC)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				continue
			}
			elapsed = commitTime.Sub(beforeCommitTime)
			// If still negative, clamp to zero
			if elapsed < 0 {
				elapsed = 0
			}
		}
		var totalDelta time.Duration
		h, err := time.ParseDuration(*durationPtr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		if elapsed < h*2 {
			totalDelta = elapsed
		} else {
			totalDelta = h
		}
		if *periodsPtr {
			// Extend or start an active period
			if elapsed < h*2 && !usedFallback {
				if currentPeriod == nil {
					currentPeriod = &ActivePeriod{Start: authorTime, End: authorTime, Duration: 0}
				} else {
					currentPeriod.End = authorTime
					currentPeriod.Duration += elapsed
				}
			} else {
				// Close current period and start a new one
				if *periodsPtr {
					if currentPeriod != nil {
						activePeriods = append(activePeriods, *currentPeriod)
					}
					currentPeriod = &ActivePeriod{Start: authorTime, End: authorTime, Duration: 0}
				}
			}
		}
		if *debugPtr {
			if n != 0 {
				fmt.Fprintf(os.Stdout, "%s (+%s) >\n", elapsed, totalDelta)
			}
			fmt.Println("\t", strings.ReplaceAll(l, "|", " "))
		}
		total += totalDelta
		beforeCommitTime = authorTime
	}
	// After loop, if verbose and period is open, close it
	if *periodsPtr && currentPeriod != nil {
		activePeriods = append(activePeriods, *currentPeriod)
	}
	if *periodsPtr {
		fmt.Println("Active periods:")
		for i, p := range activePeriods {
			fmt.Printf("  %2d. %s -> %s : %s\n", i+1, p.Start.Format(time.RFC3339), p.End.Format(time.RFC3339), p.Duration)
		}
	}
	fmt.Printf("From %q to %q : %s\n", *sincePtr, *beforePtr, total)
}
