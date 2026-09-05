package daemon

import (
	"strconv"

	"github.com/Alex4386/komitake/internal/config"
)

func ffmpegVideoArguments(profile VideoProfile) []string {
	prefix := []string{"-hide_banner", "-loglevel", "verbose", "-nostats"}
	if profile.Backend == config.VideoHwaccelCustom {
		arguments := append([]string{}, prefix...)
		arguments = append(arguments, profile.Video.FFmpegArgs.Input...)
		arguments = append(arguments, profile.Video.FFmpegArgs.Output...)
		return arguments
	}

	return applyFFmpegArgOverrides(profile, applyFFmpegProfile(profile, ffmpegBuiltinArguments(profile)))
}

func applyFFmpegProfile(profile VideoProfile, builtIn []string) []string {
	profileName := profile.Video.NormalizedFFmpegProfile()
	if profileName == "" || profile.Backend == config.VideoHwaccelCustom {
		return builtIn
	}
	inputArgs, outputArgs := ffmpegNamedProfileArguments(profileName, profile.Backend)
	if len(inputArgs) == 0 && len(outputArgs) == 0 {
		return builtIn
	}
	inputSide, inputSpec, outputSide := splitFFmpegArgumentsAtPipeInput(builtIn)
	inputSide = mergeFFmpegArguments(inputSide, inputArgs)
	outputSide = mergeFFmpegArguments(outputSide, outputArgs)
	arguments := append([]string(nil), inputSide...)
	arguments = append(arguments, inputSpec...)
	return append(arguments, outputSide...)
}

func ffmpegNamedProfileArguments(profileName, backend string) (input, output []string) {
	switch profileName {
	case config.VideoFFmpegProfileRealtime:
		input = []string{"-fflags", "nobuffer", "-flags", "low_delay"}
		switch backend {
		case config.VideoHwaccelVAAPI:
			output = []string{"-low_delay", "1"}
		case config.VideoHwaccelNVENC:
			output = []string{"-preset", "p1", "-tune", "ull", "-rc-lookahead", "0", "-delay", "0"}
		case config.VideoHwaccelQSV:
			output = []string{"-look_ahead", "0"}
		case config.VideoHwaccelNone:
			output = []string{"-preset", "ultrafast", "-tune", "zerolatency"}
		}
	}
	return input, output
}

func ffmpegBuiltinArguments(profile VideoProfile) []string {
	prefix := []string{"-hide_banner", "-loglevel", "verbose", "-nostats"}
	input := []string{"-f", "h264", "-framerate", strconv.Itoa(ffmpegVideoFrameRate), "-i", "pipe:0", "-an"}
	tail := []string{
		"-async_depth", "1", "-bf", "0", "-g", strconv.Itoa(ffmpegVideoGOPFrames),
		"-idr_interval", "0", "-aud", "0", "-profile:v", "high",
		"-bsf:v", "h264_mp4toannexb,h264_metadata=aud=insert",
		"-flush_packets", "1", "-f", "h264", "pipe:1",
	}

	var arguments []string
	switch profile.Backend {
	case config.VideoHwaccelVAAPI:
		arguments = append(arguments, prefix...)
		arguments = append(arguments,
			"-init_hw_device", "vaapi=va:"+profile.RenderNode, "-filter_hw_device", "va",
			"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi",
		)
		arguments = append(arguments, input...)
		arguments = append(arguments,
			"-c:v", profile.Encoder,
			"-low_power", "1",
			"-rc_mode", "CQP", "-qp", "24",
		)
	case config.VideoHwaccelNVENC:
		arguments = append(arguments, prefix...)
		arguments = append(arguments, input...)
		arguments = append(arguments,
			"-c:v", profile.Encoder,
			"-preset", "p4",
			"-rc", "constqp", "-qp", "24",
		)
	case config.VideoHwaccelQSV:
		arguments = append(arguments, prefix...)
		arguments = append(arguments,
			"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw",
			"-hwaccel", "qsv", "-hwaccel_output_format", "qsv",
		)
		arguments = append(arguments, input...)
		arguments = append(arguments,
			"-c:v", profile.Encoder,
			"-global_quality", "24",
		)
	case config.VideoHwaccelNone:
		arguments = append(arguments, prefix...)
		arguments = append(arguments, input...)
		arguments = append(arguments,
			"-c:v", profile.Encoder,
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-crf", "23",
		)
	default:
		arguments = append(arguments, prefix...)
		arguments = append(arguments, input...)
		arguments = append(arguments, "-c:v", profile.Encoder)
	}
	return append(arguments, tail...)
}

func applyFFmpegArgOverrides(profile VideoProfile, builtIn []string) []string {
	if !profile.Video.HasFFmpegArgs() {
		return builtIn
	}
	inputSide, inputSpec, outputSide := splitFFmpegArgumentsAtPipeInput(builtIn)
	inputSide = mergeFFmpegArguments(inputSide, profile.Video.FFmpegArgs.Input)
	outputSide = mergeFFmpegArguments(outputSide, profile.Video.FFmpegArgs.Output)
	arguments := append([]string(nil), inputSide...)
	arguments = append(arguments, inputSpec...)
	return append(arguments, outputSide...)
}

func splitFFmpegArgumentsAtPipeInput(arguments []string) (inputSide, inputSpec, outputSide []string) {
	for index := 0; index < len(arguments); index++ {
		if arguments[index] != "-i" || index+1 >= len(arguments) || arguments[index+1] != "pipe:0" {
			continue
		}
		inputSide = append([]string(nil), arguments[:index]...)
		inputSpec = []string{"-i", "pipe:0"}
		next := index + 2
		if next < len(arguments) && arguments[next] == "-an" {
			inputSpec = append(inputSpec, "-an")
			next++
		}
		outputSide = append([]string(nil), arguments[next:]...)
		return inputSide, inputSpec, outputSide
	}
	return append([]string(nil), arguments...), nil, nil
}

// mergeFFmpegArguments appends overrides after the built-in profile so ffmpeg
// last-wins semantics apply to duplicate flags like -qp or -framerate.
func mergeFFmpegArguments(base, overrides []string) []string {
	if len(overrides) == 0 {
		return base
	}
	merged := append([]string(nil), base...)
	return append(merged, overrides...)
}
