package main

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/jonas747/dca"
)

var commands = []string{
	"!audio",
	"!stop",
	"!clear",
	"!frase",
	"!jogo",
}

var done = make(chan error)

var audioMap = make(map[string]string)

func readAudioConfig(configPath string) {
	file, err := os.Open(configPath)
	if err != nil {
		fmt.Println("Could not read file: ", err)
		return
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanWords)

	var words []string

	for scanner.Scan() {
		words = append(words, scanner.Text())
	}

	for i := 0; i < len(words)-1; i += 2 {
		audioMap[words[i]] = words[i+1]
	}
}

func playSound(s *discordgo.Session, guildID, channelID string, audiofile string) (err error) {
	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		fmt.Println("Could not join voice channel: ", err)
		return err
	}

	vc.Speaking(true)
	defer vc.Speaking(false)
	defer vc.Disconnect()

	opts := dca.StdEncodeOptions
	opts.RawOutput = true
	opts.Bitrate = 120

	encodeSession, err := dca.EncodeFile(audiofile, opts)
	if err != nil {
		fmt.Println("Could not create encode session: ", err)
		return err
	}

	done = make(chan error)
	stream := dca.NewStream(encodeSession, vc, done)
	ticker := time.NewTicker(time.Second)

	for {
		select {
		case err := <-done:
			if err != nil && err != io.EOF {
				fmt.Println("An error occured", err)
				return err
			}
			encodeSession.Truncate()
			return nil

		case <-ticker.C:
			stats := encodeSession.Stats()
			playbackPosition := stream.PlaybackPosition()
			fmt.Printf("Playback: %10s, Transcode Stats: Time: %5s, Size: %5dkB, Bitrate: %6.2fkB, Speed: %5.1fx\n",
				playbackPosition, stats.Duration.String(), stats.Size, stats.Bitrate, stats.Speed)
		}
	}
}

func joinVoice(s *discordgo.Session, m *discordgo.MessageCreate, audiofile string) {
	c, err := s.State.Channel(m.ChannelID)
	if err != nil {
		fmt.Println("Could not find channel: ", err)
		return
	}

	g, err := s.State.Guild(c.GuildID)
	if err != nil {
		fmt.Println("Could not find guild: ", err)
		return
	}

	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {
			err = playSound(s, g.ID, vs.ChannelID, audiofile)
			if err != nil {
				fmt.Println("Could not play sound: ", err)
				return
			}

			return
		}
	}
}

func messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	split := strings.Split(m.Content, " ")
	if len(split) == 0 {
		return
	}

	if split[0] == "!audio" {
		if val, ok := audioMap[split[1]]; ok {
			joinVoice(s, m, val)
		}
	}

	if split[0] == "!stop" {
		done <- fmt.Errorf("Stop")
	}

	if split[0] == "!clear" {
		clearMessages(s, m)
	}

	if split[0] == "!frase" {
		msg := GeraFrase()
		sendMessage(s, m, msg)
	}

	if split[0] == "!frasetts" {
		msg := GeraFrase()
		sendMessageTTS(s, m, msg)
	}

	if split[0] == "!jogo" {
		jogo := GeraJogo()
		changeGame(s, m, jogo)
	}
}

func changeGame(s *discordgo.Session, m *discordgo.MessageCreate, game string) {
	err := s.UpdateStatus(0, game)
	if err != nil {
		fmt.Println("Could not update status: ", err)
	}
}

func sendMessage(s *discordgo.Session, m *discordgo.MessageCreate, msg string) {
	_, err := s.ChannelMessageSend(m.ChannelID, msg)
	if err != nil {
		fmt.Println("Could not send message: ", err)
	}
}

func sendMessageTTS(s *discordgo.Session, m *discordgo.MessageCreate, msg string) {
	_, err := s.ChannelMessageSendTTS(m.ChannelID, msg)
	if err != nil {
		fmt.Println("Could not send message tts: ", err)
	}
}

func clearMessages(s *discordgo.Session, m *discordgo.MessageCreate) {
	ids := make([]string, 0)

	messages, err := s.ChannelMessages(m.ChannelID, 100, "", "", "")
	if err != nil {
		fmt.Println("Could not get channel messages: ", err)
		return
	}

	fmt.Printf("Found %d messages\n", len(messages))

	for _, message := range messages {
		if message.Author.ID == s.State.User.ID {
			ids = append(ids, message.ID)
		} else {
			for _, cmd := range commands {
				if strings.HasPrefix(message.Content, cmd) {
					ids = append(ids, message.ID)
					break
				}
			}
		}
	}

	fmt.Printf("Attempting to delete %d messages\n", len(ids))

	err = s.ChannelMessagesBulkDelete(m.ChannelID, ids)
	if err != nil {
		fmt.Println("Could not delete messages")
	}
}

func main() {
	token := os.Getenv("LUQUITO_BOT")
	if len(token) == 0 {
		fmt.Println("No token found")
		return
	}

	readAudioConfig("audio-config.txt")

	rand.Seed(time.Now().UnixNano())

	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		panic(err)
	}

	discord.AddHandler(messageHandler)
	discord.State.MaxMessageCount = 100

	fmt.Println("Connecting...")

	err = discord.Open()
	if err != nil {
		panic(err)
	}

	fmt.Println("Ready!")

	sc := make(chan os.Signal, 1)

	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	fmt.Println("Closing...")
	discord.Close()
}