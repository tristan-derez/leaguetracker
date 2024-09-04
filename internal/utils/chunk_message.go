package utils

import "strings"

func ChunkMessage(message string, maxLength int) []string {
	var chunks []string
	lines := strings.Split(message, "\n")
	currentChunk := ""

	for _, line := range lines {
		if len(currentChunk)+len(line)+1 > maxLength {
			if currentChunk != "" {
				chunks = append(chunks, strings.TrimSpace(currentChunk))
				currentChunk = ""
			}
			if len(line) > maxLength {
				for len(line) > maxLength {
					chunks = append(chunks, line[:maxLength])
					line = line[maxLength:]
				}
			}
		}
		if currentChunk != "" {
			currentChunk += "\n"
		}
		currentChunk += line
	}

	if currentChunk != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	return chunks
}
