package virtualbox

import (
	"bufio"
	"strings"
)

type Parser struct {
	content string
}

func (p *Parser) asList() (result []map[string]string) {
	scanner := bufio.NewScanner(strings.NewReader(p.content))
	current := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			result = append(result, current)
			current = make(map[string]string)
		} else {
			splitted := strings.Split(line, ":")
			current[splitted[0]] = strings.TrimSpace(splitted[1])
		}
	}
	if len(current) > 0 {
		result = append(result, current)
	}
	return
}

func (p *Parser) asMap() map[string]string {
	scanner := bufio.NewScanner(strings.NewReader(p.content))
	result := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=") {
			splitted := strings.Split(line, "=")
			key := strings.Trim(splitted[0], "\"")
			value := strings.Trim(splitted[1], "\"")
			result[key] = value
		}
	}
	return result
}
