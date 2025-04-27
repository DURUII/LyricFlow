package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// LyricLine represents a lyric with start/end times.
type LyricLine struct {
	Start float64 // start time in seconds
	End   float64 // end time in seconds
	Text  string  // lyric text
}

// StyleOptions holds styling and audio mix settings.
type StyleOptions struct {
	// Video text/style
	Title                   string // main title
	Subtitle                string // artist subtitle
	Bullet                  string // bullet line (first selected lyric)
	TitleColor              string // color of title
	SubtitleColor           string // color of subtitle
	BulletColor             string // color of bullet
	NormalColor             string // lyric text color
	HighlightColor          string // highlight text color
	FinishedColor           string
	CheckboxSelectedColor   string // selected checkbox color
	CheckboxUnselectedColor string // unselected checkbox color
	FontPath                string // path to TTF font
}

func main() {
	// File paths
	audioOriginal := "assets/audio/颜人中 - 晚安.mp3"
	audioBacking := "assets/audio/颜人中 - 晚安 (伴奏).mp3"
	lyricsFile := "assets/lyrics/颜人中 - 晚安.lrc"
	fontPath := "assets/fonts/MiSans-Regular.ttf"
	outputFile := "output/result.mp4"

	ensureDirs()

	// Parse lyrics
	allLyrics, err := parseLRC(lyricsFile)
	if err != nil {
		panic(err)
	}

	// User selects start/end lines
	//selected := askUserSelect(allLyrics)
	selected := allLyrics[25:33]
	if len(selected) < 1 {
		panic("no lyrics selected")
	}

	// Style & audio options
	artist := extractMetadataArtist(audioOriginal)
	opts := StyleOptions{
		Title:                   strings.Trim(strings.Split(extractMetadataTitle(audioOriginal), "-")[1], " 》《"),
		Subtitle:                artist,
		Bullet:                  selected[0].Text,
		TitleColor:              "white",
		SubtitleColor:           "white",
		BulletColor:             "white",
		NormalColor:             "#8E8E93",
		HighlightColor:          "#f4ea2a",
		FinishedColor:           "white",
		CheckboxSelectedColor:   "#f4ea2a",
		CheckboxUnselectedColor: "#8E8E93",
		FontPath:                fontPath,
	}

	// Compute offset and duration
	offset := selected[0].Start
	duration := selected[len(selected)-1].End - offset + 2.0

	// 1) Generate background video
	bg := "output/background.mp4"
	generateBackground(bg, duration)

	// 2) Mix audio with weighted gains
	audioMixed := "output/audio_mixed.mp3"
	mergeAudioTracksWithAccompaniment(
		audioOriginal,
		audioBacking,
		selected,
		0.2,  // 原唱低音量，比如 20%
		0.75, // 伴奏高音量，比如 100%
		audioMixed,
	)

	// 3) Overlay lyrics & checkboxes
	videoNoAudio := "output/final_video.mp4"
	generateLyricsVideoNotesStyle(bg, selected, opts, offset, videoNoAudio)

	// 4) Combine video + mixed audio
	composeFinalVideo(videoNoAudio, audioMixed, outputFile)

	fmt.Println("Done:", outputFile)
}

// ensureDirs creates required directories.
func ensureDirs() {
	os.MkdirAll("output", 0755)
	os.MkdirAll("assets/audio", 0755)
	os.MkdirAll("assets/lyrics", 0755)
	os.MkdirAll("assets/fonts", 0755)
}

// parseLRC reads an .lrc file into LyricLine slices.
func parseLRC(path string) ([]LyricLine, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []LyricLine
	re := regexp.MustCompile(`\[(\d+):(\d+\.\d+)\](.*)`)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		m := re.FindStringSubmatch(scanner.Text())
		if len(m) != 4 {
			continue
		}
		min, _ := strconv.Atoi(m[1])
		sec, _ := strconv.ParseFloat(m[2], 64)
		start := float64(min)*60 + sec
		text := strings.TrimSpace(m[3])
		lines = append(lines, LyricLine{Start: start, Text: text})
	}
	for i := range lines {
		if i < len(lines)-1 {
			lines[i].End = lines[i+1].Start
		} else {
			lines[i].End = lines[i].Start + 5.0
		}
	}
	return lines, nil
}

// askUserSelect prompts for start/end line numbers.
func askUserSelect(lyrics []LyricLine) []LyricLine {
	fmt.Println("Available lyrics:")
	for i, l := range lyrics {
		fmt.Printf("%2d: %s\n", i+1, l.Text)
	}
	fmt.Println("Enter start and end line (inclusive), e.g. \"5 12\":")
	var start, end int
	_, err := fmt.Scan(&start, &end)
	if err != nil || start < 1 || end < start || end > len(lyrics) {
		panic("invalid input")
	}
	return lyrics[start-1 : end]
}

// extractMetadataArtist reads the artist tag from an MP3.
func extractMetadataArtist(path string) string {
	out, _ := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format_tags=artist",
		"-of", "default=noprint_wrappers=1", path).Output()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "TAG:artist=") {
			return strings.TrimPrefix(line, "TAG:artist=")
		}
	}
	return ""
}

// extractMetadataTitle reads artist & title tags and formats them.
func extractMetadataTitle(path string) string {
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format_tags=artist,title",
		"-of", "default=noprint_wrappers=1", path).Output()
	if err != nil {
		return "《Unknown》"
	}
	artist, title := "", ""
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "TAG:artist=") {
			artist = strings.TrimPrefix(line, "TAG:artist=")
		}
		if strings.HasPrefix(line, "TAG:title=") {
			title = strings.TrimPrefix(line, "TAG:title=")
		}
	}
	if artist != "" && title != "" {
		return fmt.Sprintf("《%s - %s》", artist, title)
	} else if title != "" {
		return fmt.Sprintf("《%s》", title)
	}
	return "《Unknown》"
}

// generateBackground makes a solid black video of given duration.
func generateBackground(output string, duration float64) {
	run(exec.Command("ffmpeg", "-y", "-f", "lavfi",
		"-i", fmt.Sprintf("color=c=black:s=720x1280:d=%.2f", duration),
		output))
}

// mergeAudioTracksWithAccompaniment trims the first lyric segment entirely from the original track,
// and for the remaining segments mixes the original at a low gain with the backing track at a high gain,
// then concatenates all segments into one continuous audio stream.
func mergeAudioTracksWithAccompaniment(orig, back string, sel []LyricLine,
	origGain, backGain float64, output string) {
	// Build filter_complex dynamically
	var filters []string
	// 1) First segment: original only
	first := sel[0]
	filters = append(filters,
		fmt.Sprintf(
			"[0:a]atrim=%.2f:%.2f,asetpts=PTS-STARTPTS,volume=%.2f[first]",
			first.Start, first.End, 1.0, // keep full volume for first line
		))
	// 2) Remaining segments: mix original(low) + backing(high)
	for i := 1; i < len(sel); i++ {
		ln := sel[i]
		// trim backing
		filters = append(filters,
			fmt.Sprintf(
				"[1:a]atrim=%.2f:%.2f,asetpts=PTS-STARTPTS,volume=%.2f[back%d]",
				ln.Start, ln.End, backGain, i))
		// trim original
		filters = append(filters,
			fmt.Sprintf(
				"[0:a]atrim=%.2f:%.2f,asetpts=PTS-STARTPTS,volume=%.2f[orig%d]",
				ln.Start, ln.End, origGain, i))
		// amix the two
		filters = append(filters,
			fmt.Sprintf(
				"[back%d][orig%d]amix=inputs=2:duration=first[seg%d]",
				i, i, i))
	}
	// 3) Concatenate all segments: first + seg1 + seg2 + ...
	var segNames []string
	segNames = append(segNames, "[first]")
	for i := 1; i < len(sel); i++ {
		segNames = append(segNames, fmt.Sprintf("[seg%d]", i))
	}
	filters = append(filters,
		strings.Join(segNames, "")+
			fmt.Sprintf("concat=n=%d:v=0:a=1[out]", len(segNames)),
	)

	cmd := exec.Command("ffmpeg", "-y",
		"-i", orig,
		"-i", back,
		"-filter_complex", strings.Join(filters, ";"),
		"-map", "[out]",
		"-acodec", "libmp3lame",
		output,
	)
	run(cmd)
}

func generateLyricsVideoNotesStyle(
	background string,
	lyrics []LyricLine,
	opts StyleOptions,
	offset float64,
	output string,
) {
	var filterParts []string
	inputs := []string{
		"-i", background, // 0:v 背景
		"-i", "assets/img/选择.png", // 1:v 未选中
		"-i", "assets/img/选择-勾选.png", // 2:v 已勾选
	}

	// 起始流
	current := "[0:v]"
	baseIdx := 0

	startY, lineH := 340, 72

	// Step 1：添加标题、子标题、Bullet
	filterParts = append(filterParts, fmt.Sprintf(
		"%sdrawtext=fontfile=%s:text='%s':fontsize=48:fontcolor=%s:x=(w-text_w)/2:y=100[base%d]",
		current, opts.FontPath, escape(opts.Title), opts.TitleColor, baseIdx+1))
	current = fmt.Sprintf("[base%d]", baseIdx+1)

	filterParts = append(filterParts, fmt.Sprintf(
		"%sdrawtext=fontfile=%s:text='-%s':fontsize=36:fontcolor=%s:x=(w-text_w)/2:y=180[base%d]",
		current, opts.FontPath, escape(opts.Subtitle), opts.SubtitleColor, baseIdx+2))
	current = fmt.Sprintf("[base%d]", baseIdx+2)

	filterParts = append(filterParts, fmt.Sprintf(
		"%sdrawtext=fontfile=%s:text='• %s':fontsize=36:fontcolor=%s:x=60:y=260[base%d]",
		current, opts.FontPath, escape(opts.Bullet), opts.BulletColor, baseIdx+3))
	current = fmt.Sprintf("[base%d]", baseIdx+3)

	// Step 2：Checklist每行
	for i, ln := range lyrics[1:] {
		y := startY + i*lineH
		start := ln.Start - offset
		end := ln.End - offset

		// 缩放未选中图并叠加
		filterParts = append(filterParts, fmt.Sprintf(
			"[1:v]scale=40:40[unchecked%d];%s[unchecked%d]overlay=x=60:y=%d:enable='lt(t,%.2f)'[base%d]",
			i, current, i, y, start, baseIdx+4+i*5,
		))
		current = fmt.Sprintf("[base%d]", baseIdx+4+i*5)

		// 缩放已选中图并叠加
		filterParts = append(filterParts, fmt.Sprintf(
			"[2:v]scale=40:40[checked%d];%s[checked%d]overlay=x=60:y=%d:enable='gte(t,%.2f)'[base%d]",
			i, current, i, y, start, baseIdx+5+i*5,
		))
		current = fmt.Sprintf("[base%d]", baseIdx+5+i*5)

		// 未唱到（灰色）
		filterParts = append(filterParts, fmt.Sprintf(
			"%sdrawtext=fontfile=%s:text='%s':fontsize=36:fontcolor=%s:x=120:y=%d:enable='lt(t,%.2f)'[base%d]",
			current, opts.FontPath, escape(ln.Text), opts.NormalColor, y+4, start, baseIdx+6+i*5,
		))
		current = fmt.Sprintf("[base%d]", baseIdx+6+i*5)

		// 正在唱（高亮色）
		filterParts = append(filterParts, fmt.Sprintf(
			"%sdrawtext=fontfile=%s:text='%s':fontsize=36:fontcolor=%s:x=120:y=%d:enable='between(t,%.2f,%.2f)'[base%d]",
			current, opts.FontPath, escape(ln.Text), opts.HighlightColor, y+4, start, end, baseIdx+7+i*5,
		))
		current = fmt.Sprintf("[base%d]", baseIdx+7+i*5)

		// 唱完（白色）
		filterParts = append(filterParts, fmt.Sprintf(
			"%sdrawtext=fontfile=%s:text='%s':fontsize=36:fontcolor=%s:x=120:y=%d:enable='gt(t,%.2f)'[base%d]",
			current, opts.FontPath, escape(ln.Text), opts.FinishedColor, y+4, end, baseIdx+8+i*5,
		))
		current = fmt.Sprintf("[base%d]", baseIdx+8+i*5)
	}

	final := current

	cmdArgs := []string{"-y"}
	cmdArgs = append(cmdArgs, inputs...)
	cmdArgs = append(cmdArgs,
		"-filter_complex", strings.Join(filterParts, ";"),
		"-map", final,
		"-c:v", "libx264", "-preset", "fast", "-crf", "18",
		output,
	)

	cmd := exec.Command("ffmpeg", cmdArgs...)
	run(cmd)
}

// composeFinalVideo merges the video (no audio) with the mixed audio file.
func composeFinalVideo(video, audio, output string) {
	cmd := exec.Command("ffmpeg", "-y",
		"-i", video, "-i", audio,
		"-c:v", "copy", "-c:a", "aac", "-shortest", output)
	run(cmd)
}

// escape prepares text for ffmpeg drawtext by escaping special chars.
func escape(s string) string {
	s = strings.ReplaceAll(s, ":", "\\:")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, ",", "\\,")
	return s
}

// run executes a command, streaming its output to stdout/stderr, and panics on error.
func run(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
