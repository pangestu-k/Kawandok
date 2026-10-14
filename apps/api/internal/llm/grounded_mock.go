package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const groundedMockName = "mock-grounded"

type GroundedMock struct {
	plain *Mock
}

func NewGroundedMock() *GroundedMock {
	return &GroundedMock{plain: NewMock()}
}

func (g *GroundedMock) Name() string {
	return groundedMockName
}

func (g *GroundedMock) Complete(ctx context.Context, req CompleteRequest) (CompleteResponse, error) {
	if err := ctx.Err(); err != nil {
		return CompleteResponse{}, err
	}
	if err := req.Validate(); err != nil {
		return CompleteResponse{}, err
	}
	last := req.Messages[len(req.Messages)-1]
	reply := groundedReply(last.Content)
	resp := CompleteResponse{
		Message: Message{Role: RoleAssistant, Content: reply},
		Usage: Usage{
			InputTokens:  estimateInputTokens(req),
			OutputTokens: len(strings.Fields(reply)),
		},
		FinishReason: "stop",
	}
	return resp, nil
}

func (g *GroundedMock) Stream(ctx context.Context, req CompleteRequest, fn StreamFunc) (CompleteResponse, error) {
	if fn == nil {
		return CompleteResponse{}, fmt.Errorf("%w: stream function is nil", ErrInvalidRequest)
	}
	resp, err := g.Complete(ctx, req)
	if err != nil {
		return CompleteResponse{}, err
	}
	content := resp.Message.Content
	for i := 0; i < len(content); i += mockStreamStride {
		if err := ctx.Err(); err != nil {
			return CompleteResponse{}, err
		}
		end := i + mockStreamStride
		if end > len(content) {
			end = len(content)
		}
		if err := fn(content[i:end]); err != nil {
			return CompleteResponse{}, err
		}
	}
	return resp, nil
}

const uncertaintyReply = "Saya tidak dapat menemukan jawaban dari dokumen yang tersedia. " +
	"Dokumen yang relevan dengan pertanyaan Anda tidak ditemukan atau tidak dapat Anda akses."

const evidenceStartTag = "[EVIDENCE "

type evidenceBlock struct {
	index   int
	id      string
	content string
}

func groundedReply(prompt string) string {
	blocks := parseEvidenceBlocks(prompt)
	if len(blocks) == 0 {
		return uncertaintyReply
	}

	var citations []string
	for _, b := range blocks {
		citations = append(citations, fmt.Sprintf("[%d]", b.index))
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].index < blocks[j].index })

	var sb strings.Builder
	sb.WriteString("Berdasarkan dokumen yang dapat Anda akses: ")
	first := blocks[0]
	snippet := first.content
	if len(snippet) > 160 {
		snippet = snippet[:160]
	}
	fmt.Fprintf(&sb, "%s %s", snippet, strings.Join(citations, ""))
	if len(blocks) > 1 {
		fmt.Fprintf(&sb, "Informasi tersebut berasal dari %d bagian dokumen yang relevan.", len(blocks))
	}
	return sb.String()
}

func parseEvidenceBlocks(prompt string) []evidenceBlock {
	var blocks []evidenceBlock
	remaining := prompt
	for {
		start := strings.Index(remaining, evidenceStartTag)
		if start < 0 {
			break
		}
		rest := remaining[start+len(evidenceStartTag):]
		endTag := strings.Index(rest, "]\n")
		if endTag < 0 {
			break
		}
		header := rest[:endTag]
		bodyStart := endTag + 2
		bodyEnd := strings.Index(rest[bodyStart:], evidenceStartTag)
		var body string
		if bodyEnd < 0 {
			body = rest[bodyStart:]
		} else {
			body = rest[bodyStart : bodyStart+bodyEnd]
		}

		id := header
		if eq := strings.Index(header, " id="); eq >= 0 {
			id = header[eq+4:]
		}
		content := strings.TrimSpace(strings.TrimSuffix(body, "\n"))
		if content != "" {
			blocks = append(blocks, evidenceBlock{id: id, content: content})
		}
		remaining = rest[bodyStart+len(body):]
	}

	for i := range blocks {
		blocks[i].index = i + 1
	}
	return blocks
}
