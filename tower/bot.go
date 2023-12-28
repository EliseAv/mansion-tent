package tower

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
)

type botIds struct {
	guild, channel, dm string
}

type bot struct {
	session *discordgo.Session
	ids     botIds
}

var Bot bot

func init() {
	s, err := discordgo.New("Bot " + os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}
	Bot.session = s
	Bot.ids = botIds{
		guild:   os.Getenv("GUILD_ID"),
		channel: os.Getenv("CHANNEL_ID"),
		dm:      os.Getenv("DM_CHANNEL_ID"),
	}
	s.AddHandler(Bot.onReady)
	s.AddHandler(Bot.onInteractionGo)
	//s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentMessageContent
}

func (b *bot) Run() {
	err := b.session.Open()
	if err != nil {
		log.Fatalf("Error opening connection: %v", err)
	}
	b.setCommands()

	defer b.session.Close()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	log.Println("Bot is now running. Press CTRL-C to exit.")
	<-stop
}

func (b *bot) setCommands() {
	uid := b.session.State.User.ID
	registeredCommands, _ := b.session.ApplicationCommands(uid, b.ids.guild)
	for _, v := range registeredCommands {
		b.session.ApplicationCommandDelete(uid, b.ids.guild, v.ID)
	}
	applicationCommand := &discordgo.ApplicationCommand{
		Name:        "factorio",
		Description: "Start the Factorio server",
	}
	b.session.ApplicationCommandCreate(uid, b.ids.dm, applicationCommand)
	b.session.ApplicationCommandCreate(uid, b.ids.channel, applicationCommand)
}

func (b *bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("Bot is up as %v#%v!\n", s.State.User.Username, s.State.User.Discriminator)
}

func (b *bot) onInteractionGo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	go b.onInteraction(s, i)
}

func (b *bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	commandData := i.ApplicationCommandData()
	log.Printf("Interaction received... %v\n", commandData)
	if commandData.Name == "factorio" {
		b.onCommandFactorio(s, i)
	}
}

func (b *bot) onCommandFactorio(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.ChannelID != b.ids.channel && i.ChannelID != b.ids.dm {
		b.replyQuick(i, "This command can only be used in a specific channel.")
		log.Printf("Command factorio received in a wrong channel: %v\n", i.ChannelID)
		return
	}
	b.replyLater(i)
	Dispatcher.LaunchFactorio()
	if Dispatcher.err != nil {
		b.replyAmend(i, "Error: "+Dispatcher.err.Error())
	} else {
		b.replyAmend(i, fmt.Sprintf(
			"Factorio server starting at `%s` (`%s`)",
			os.Getenv("ROUTE53_FQDN"), Dispatcher.ip))
	}
}

func (b *bot) replyQuick(i *discordgo.InteractionCreate, content string) {
	ir := discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content},
	}
	b.session.InteractionRespond(i.Interaction, &ir)
}

func (b *bot) replyLater(i *discordgo.InteractionCreate) {
	ir := discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}
	b.session.InteractionRespond(i.Interaction, &ir)
}

func (b *bot) replyAmend(i *discordgo.InteractionCreate, content string) {
	b.session.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
}