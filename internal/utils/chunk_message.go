package utils

func ChunkMessage(message string, chunkSize int) []string {
	if len(message) <= chunkSize {
		return []string{message}
	}

	var chunks []string
	for start := 0; start < len(message); start += chunkSize {
		end := start + chunkSize
		if end > len(message) {
			end = len(message)
		}
		chunks = append(chunks, message[start:end])
	}
	return chunks
}
