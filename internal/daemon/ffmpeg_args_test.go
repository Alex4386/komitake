package daemon

import "testing"

func TestSplitFFmpegArgumentsAtPipeInput(t *testing.T) {
	builtIn := []string{
		"-hide_banner",
		"-init_hw_device", "vaapi=va:/dev/dri/renderD128",
		"-f", "h264", "-framerate", "25", "-i", "pipe:0", "-an",
		"-c:v", "h264_vaapi", "-qp", "24", "-f", "h264", "pipe:1",
	}
	inputSide, inputSpec, outputSide := splitFFmpegArgumentsAtPipeInput(builtIn)
	if len(inputSpec) != 3 || inputSpec[0] != "-i" || inputSpec[1] != "pipe:0" || inputSpec[2] != "-an" {
		t.Fatalf("inputSpec = %v", inputSpec)
	}
	if !containsArgumentPair(inputSide, "-f", "h264") || containsArgumentPair(inputSide, "-i", "pipe:0") {
		t.Fatalf("inputSide = %v", inputSide)
	}
	if !containsArgumentPair(outputSide, "-c:v", "h264_vaapi") || !containsArgumentPair(outputSide, "-f", "h264") {
		t.Fatalf("outputSide = %v", outputSide)
	}
}

func indexPair(arguments []string, key, value string) int {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == key && arguments[index+1] == value {
			return index
		}
	}
	return -1
}

func lastArgumentValue(arguments []string, key string) string {
	for index := len(arguments) - 2; index >= 0; index-- {
		if arguments[index] == key {
			return arguments[index+1]
		}
	}
	return ""
}
