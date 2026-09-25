package taskdump

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reTaskIdQuery = regexp.MustCompile(`(?i)[?&]task_id=(\d+)`)
	reTaskPath    = regexp.MustCompile(`/task/(\d+)`)
	reLastNumber  = regexp.MustCompile(`(\d+)\D*$`)
)

// ParseTaskId accepts the ways a task is usually referred to:
//
//	123
//	#123, #KB123, KB123, kb-123
//	https://kanboard.example.com/task/123
//	https://kanboard.example.com/?controller=task&action=show&task_id=123&project_id=1
func ParseTaskId(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty task reference")
	}
	if n, err := strconv.Atoi(s); err == nil {
		return checkId(n)
	}
	for _, re := range []*regexp.Regexp{reTaskIdQuery, reTaskPath, reLastNumber} {
		if m := re.FindStringSubmatch(s); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				return 0, fmt.Errorf("invalid task reference %q", s)
			}
			return checkId(n)
		}
	}
	return 0, fmt.Errorf("cannot find a task number in %q", s)
}

func checkId(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("task id must be a positive integer, got %v", n)
	}
	return n, nil
}
