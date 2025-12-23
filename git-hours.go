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

func main() {
	since, before := beforeMonth()
	sincePtr := flag.String("since", since+" 00:00:00 "+timeZoneOffset(), "since(after) date")
	beforePtr := flag.String("before", before+" 23:59:59 "+timeZoneOffset(), "before date")
	authorPtr := flag.String("author", "", "author name")            // git option : --author="\(Adam\)\|\(Jon\)"
	durationPtr := flag.String("duration", "1h", "git log duration") // default "1h"
	debugPtr := flag.Bool("debug", false, "debug mode")
	helpPtr := flag.Bool("help", false, "print help")
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
	cmd := exec.Command(
		"git",
		"--no-pager",
		"log",
		"--reverse",
		"--date=iso-local",
		`--pretty=format:%ad|%cd|%an|%s`,
		fmt.Sprintf(`--author=%s`, author),
		fmt.Sprintf(`--since="%s"`, *sincePtr),
		fmt.Sprintf(`--before="%s"`, *beforePtr),
	)
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
	for n, l := range strings.Split(stdout.String(), "\n") {
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
		if elapsed < 0 {
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
		if *debugPtr {
			if n != 0 {
				fmt.Fprintf(os.Stdout, "%s (+%s) >\n", elapsed, totalDelta)
			}
			fmt.Println("\t", l)
		}
		total += totalDelta
		beforeCommitTime = authorTime
	}
	fmt.Printf("From %q to %q : %s\n", *sincePtr, *beforePtr, total)
}
