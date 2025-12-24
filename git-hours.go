package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
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

	// Helper to build git log args
	buildGitArgs := func(extra ...string) []string {
		args := []string{"--no-pager", "log"}
		args = append(args, extra...)
		args = append(args,
			"--date=iso-local",
			`--pretty=format:%H|%ad|%cd|%an|%s`,
		)
		if *authorPtr != "" {
			args = append(args, fmt.Sprintf(`--author=%s`, author))
		}
		args = append(args,
			fmt.Sprintf(`--since="%s"`, *sincePtr),
			fmt.Sprintf(`--before="%s"`, *beforePtr),
		)
		return args
	}

	// Map to deduplicate by commit hash
	commitMap := make(map[string]string)
	var allLines []string
	var runLog = func(args []string) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd := exec.Command("git", args...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			if ee, ok := err.(*exec.Error); ok && ee.Err == exec.ErrNotFound {
				fmt.Fprintln(os.Stderr, "Error: git is not installed or not found in PATH.")
				os.Exit(1)
			}
			if stderr.String() != "" {
				fmt.Fprintf(os.Stderr, stderr.String())
			} else {
				fmt.Fprintf(os.Stderr, "Error running git: %v\n", err)
			}
			os.Exit(1)
		}
		if stderr.String() != "" {
			fmt.Fprintf(os.Stderr, stderr.String())
			os.Exit(1)
		}
		for _, l := range strings.Split(stdout.String(), "\n") {
			if l == "" {
				continue
			}
			hash := strings.SplitN(l, "|", 2)[0]
			if _, exists := commitMap[hash]; !exists {
				commitMap[hash] = l
				allLines = append(allLines, l)
			}
		}
	}

	if *allBranchesPtr && *reflogPtr {
		runLog(buildGitArgs("--all"))
		runLog(buildGitArgs("--walk-reflogs", "--all"))
	} else if *allBranchesPtr {
		runLog(buildGitArgs("--all"))
	} else if *reflogPtr {
		runLog(buildGitArgs("--walk-reflogs"))
	} else {
		runLog(buildGitArgs())
	}

	total, err := time.ParseDuration("0h")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if len(allLines) == 0 {
		fmt.Printf("From %q to %q : %s\n", *sincePtr, *beforePtr, total)
		os.Exit(0)
	}

	// Sort allLines by author date (oldest to newest)
	type commitLine struct {
		line string
		authorTime time.Time
	}
	var commitLines []commitLine
	for _, l := range allLines {
		parts := strings.SplitN(l, "|", 5)
		if len(parts) < 5 {
			continue
		}
		authorRFC, err := ISO8601ToRFC3339(findISO8601.FindString(parts[1]))
		if err != nil {
			continue
		}
		authorTime, err := time.Parse(time.RFC3339, authorRFC)
		if err != nil {
			continue
		}
		commitLines = append(commitLines, commitLine{l, authorTime})
	}
	sort.Slice(commitLines, func(i, j int) bool {
		return commitLines[i].authorTime.Before(commitLines[j].authorTime)
	})

	duration, err := time.ParseDuration(*durationPtr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	var beforeCommitTime *time.Time
	var activePeriods []ActivePeriod
	var currentPeriod *ActivePeriod
	for _, cl := range commitLines {
		l := cl.line
		// Expecting format: %H|%ad|%cd|%an|%s
		parts := strings.SplitN(l, "|", 5)
		if len(parts) < 5 {
			continue
		}
		authorDateStr := parts[1]
		commitDateStr := parts[2]
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
		var elapsed *time.Duration
		usedFallback := false
		if beforeCommitTime != nil {
			elapsedValue := authorTime.Sub(*beforeCommitTime)
			// Fallback: if elapsed is negative, use commit date instead
			if elapsedValue < 0 {
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
				elapsedValue = commitTime.Sub(*beforeCommitTime)
				// If still negative, clamp to zero
				if elapsedValue < 0 {
					elapsedValue = 0
				}
			}
			elapsed = &elapsedValue
		}
		var totalDelta time.Duration
		if elapsed != nil && *elapsed < duration*2 {
			totalDelta = *elapsed
		} else {
			totalDelta = duration
		}
		if *periodsPtr {
			// Extend or start an active period
			if elapsed != nil && *elapsed < duration*2 && !usedFallback {
				if currentPeriod == nil {
					currentPeriod = &ActivePeriod{Start: authorTime, End: authorTime, Duration: 0}
				} else {
					currentPeriod.End = authorTime
					currentPeriod.Duration += *elapsed
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
			if elapsed == nil {
				fmt.Fprintf(os.Stdout, "N/A (+%s) >\n", totalDelta)
			} else {
				fmt.Fprintf(os.Stdout, "%s (+%s) >\n", *elapsed, totalDelta)
			}
			fmt.Println("\t", strings.ReplaceAll(l, "|", " "))
		}
		total += totalDelta
		beforeCommitTime = &authorTime
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
