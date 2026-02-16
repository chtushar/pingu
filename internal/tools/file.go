package tools

import "context"

type File struct{}

func (f *File) Name() string        { return "file" }
func (f *File) Description() string { return "Read or write files" }
func (f *File) InputSchema() any    { return nil }

func (f *File) Execute(ctx context.Context, input string) (string, error) {
	return "", nil
}
