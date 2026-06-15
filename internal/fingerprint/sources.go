package fingerprint

func NewSourcesChecker(method, tempDir string, dry bool) (SourcesCheckable, error) {
	// case "none":
	// 	return NoneChecker{}, nil
	return NewTimestampChecker(tempDir, dry), nil
}
