package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"gopkg.in/yaml.v2"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var (
	CARDS_JSON_URL   = "https://api.trello.com/1/lists/5917debee7c25fa77b80cae1?fields=name&cards=open&card_fields=name,url"
	RESULT_SEPARATOR = " - "
	SINGLE_VOTE      = true
	SECRET_VOTE      = true
)

type TrelloCard struct {
	ID   string `json:id`
	Name string `json:name`
	Url  string `json:url`
}

type TrelloCardsList struct {
	ID    string       `json:id`
	Name  string       `json:name`
	Cards []TrelloCard `json:cards`
}

type Election struct {
	Name       string     `yaml:name`
	ID         string     `yaml:id`
	PollsterID int        `yaml:pollster`
	Votes      []Proposal `yaml:proposal`
	// The string is a secret function of the username, so as to protect voter identity
	Voters map[string]int `yaml:voters`
}

type Proposal struct {
	Vote        int    `yaml:vote`
	Description string `yaml:description`
}

func ElectionFromMessage(pollMessage tgbotapi.Message) Election {
	election := Election{}
	err := yaml.Unmarshal([]byte(pollMessage.Text), &election)
	if err != nil {
		fmt.Println(err)
	}
	return election
}

func createYAMLFormData(cardList *TrelloCardsList, pollsterID int) string {
	election := new(Election)
	election.Name = cardList.Name
	election.PollsterID = pollsterID
	for _, card := range cardList.Cards {
		proposal := Proposal{
			Description: card.Name,
		}
		election.Votes = append(election.Votes, proposal)
	}
	d, err := yaml.Marshal(&election)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Printf("\n--- (debug) yaml dump:\n%s", string(d))
	return string(d)
}

func closePoll(message *tgbotapi.Message) tgbotapi.EditMessageTextConfig {
	fmt.Println("messageID is %d", message.MessageID)
	editMessageTextConfig := tgbotapi.NewEditMessageText(message.Chat.ID,
		message.MessageID,
		message.Text)
	editMessageTextConfig.ReplyMarkup = nil
	return editMessageTextConfig
}

func doChat(bot tgbotapi.BotAPI, update tgbotapi.Update) {

	msgTxt := update.Message.Text
	msgParts := strings.Split(msgTxt, " ")
	fmt.Printf("\n--- (debug) «%s» command/message sent by %s", msgParts, update.Message.From)

	if msgTxt == "/start" {
		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Welcome to @votbot!\n\nThis message may not be up to date, please visite https://github.com/epfl-dojo/votbot for latest documentation.\n\nThe commands you may want to use are:\n  ◦ /newvote URL\n     where the URL point to a JSON file well formatted\n  ◦ /close\n    Only when answering to a poll message to close it; Vote options will disappear and a summarized message will pop.\n\nFell free to contact and ask stuff on https://github.com/epfl-dojo/votbot\n\n                — Have fun"))
	} else if msgTxt == "/help" {
		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Please visite https://github.com/epfl-dojo/votbot for latest documentation. Feel free to ask there !\n\nYou can try\n ```\n/newvote https://raw.githubusercontent.com/epfl-dojo/votbot/master/minimal.json\n```\n as a demo...\n\n                — Have fun"))
	} else if msgTxt == "/close" {
		if update.Message.ReplyToMessage == nil {
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Please use this command while responding to the poll you want to close."))
		} else {
			// TODO:
			// - check identity of who is closing the poll - should be the same one who opened it
			fmt.Printf("\n--- (debug) ...closing poll ID ? %d", update.Message.ReplyToMessage.MessageID)
			election := ElectionFromMessage(*update.Message.ReplyToMessage)
			if election.PollsterID != update.Message.From.ID {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "You have no right to close that poll.")
				msg.ReplyToMessageID = update.Message.MessageID
				bot.Send(msg)
				return
			}
			msg := closePoll(update.Message.ReplyToMessage)
			bot.Send(msg)
			summaryMsg := createPollSummary(*update.Message.ReplyToMessage)
			bot.Send(summaryMsg)

		}
	} else if len(msgParts) == 2 && msgParts[0] == "/newvote" {
		voteUrl, err := url.Parse(msgParts[1])
		if err != nil {
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid URL, please try again."))
		}
		if voteUrl.Scheme == "" {
			voteUrl.Scheme = "https"
		}
		if voteUrl.Host == "" {
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid URL host, please try again."))
		}
		trelloCardsList := getTrelloCards(voteUrl.String())

		votingText := createYAMLFormData(trelloCardsList, update.Message.From.ID)

		buttonMarkup := createButtonForm(trelloCardsList)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, votingText)
		msg.ReplyMarkup = buttonMarkup

		msg.ReplyToMessageID = update.Message.MessageID
		fmt.Printf("\n--- (debug) sending the form for /newpoll of message ID: %d\n", update.Message.MessageID)
		reply, _ := bot.Send(msg)
		fmt.Printf("\n--- (debug) sent as ID: %d\n", reply.MessageID)
	} else {
		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command, please try again."))
	}
}

func voterID(voter *tgbotapi.User, ballot *tgbotapi.Message) string {
	if SECRET_VOTE {
		h := fnv.New32a()
		h.Write([]byte("sel ... "))
		//		h.Write([]byte(fmt.Sprintf("%d", ballot.Chat.ID)))
		voterIDBytes := make([]byte, 64)
		binary.PutVarint(voterIDBytes, int64(voter.ID))
		h.Write(voterIDBytes)
		return fmt.Sprintf("%d", h.Sum32())
	} else {
		return voter.UserName
	}
}

func createPollSummary(message tgbotapi.Message) tgbotapi.MessageConfig {
	election := ElectionFromMessage(message)
	var bufferSummary bytes.Buffer
	bufferSummary.WriteString(fmt.Sprintf("— Closing poll \"%s\" —\n", election.Name))
	// TODO: get user's username...
	// -> bufferSummary.WriteString(fmt.Sprintf("...opened by %s\n\n", message.From.UserName))
	for _, vote := range election.Votes {
		var voteNoun string
		if vote.Vote <= 1 {
			voteNoun = "vote"
		} else {
			voteNoun = "votes"
		}
		bufferSummary.WriteString(fmt.Sprintf("  ◦ %2d %s for «%s»\n", vote.Vote, voteNoun, vote.Description))
	}
	// TODO: Option XXX wins (and handle ex-aequo results)
	// -> bufferSummary.WriteString(fmt.Sprintf("— Option \"%s\" wins —\n", election.Name))
	return tgbotapi.NewMessage(message.Chat.ID, bufferSummary.String())
}

func createUpdateResponse(update tgbotapi.Update) tgbotapi.EditMessageTextConfig {
	choice := strings.TrimLeft(update.CallbackQuery.Data, "/")
	voteID, err := strconv.Atoi(choice)
	fmt.Printf("\n--- (debug) ...for choice %d by %s (messageID %d)", voteID, update.CallbackQuery.From.UserName, update.CallbackQuery.Message.MessageID)

	if err != nil {
		fmt.Println(err)
		os.Exit(127)
	}

	election := ElectionFromMessage(*update.CallbackQuery.Message)

	election.Votes[voteID].Vote += 1
	if SINGLE_VOTE {
		previousVoteChoice, voteChoiceExists := election.Voters[voterID(update.CallbackQuery.From, update.Message)]
		if voteChoiceExists {
			election.Votes[previousVoteChoice].Vote -= 1
		}
	}
	election.Voters[voterID(update.CallbackQuery.From, update.Message)] = voteID
	proposals := []string{}
	for _, proposal := range election.Votes {
		proposals = append(proposals, proposal.Description)
	}
	newPropositions, err := yaml.Marshal(&election)
	if err != nil {
		fmt.Println(err)
		os.Exit(127)
	}
	editMessageTextConfig := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID,
		update.CallbackQuery.Message.MessageID,
		string(newPropositions))
	markup := createButtons(proposals)
	editMessageTextConfig.ReplyMarkup = &markup
	return editMessageTextConfig
}

func createButtons(buttonsText []string) tgbotapi.InlineKeyboardMarkup {
	var buttons [][]tgbotapi.InlineKeyboardButton

	for idx, choice := range buttonsText {
		cleanedChoice := strings.TrimSpace(choice)
		cleanedChoice = strings.Replace(cleanedChoice, "\n", "", -1)
		if cleanedChoice != "" {
			actionURL := "/" + fmt.Sprintf("%v", idx)
			button := tgbotapi.NewInlineKeyboardButtonData(cleanedChoice, actionURL)
			buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(button))
		}
	}
	buttonsRow := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	return buttonsRow
}

func createButtonForm(cardList *TrelloCardsList) tgbotapi.InlineKeyboardMarkup {
	var choices []string
	for _, card := range cardList.Cards {
		choices = append(choices, card.Name)
	}
	return createButtons(choices)
}

func getTrelloCards(url string) *TrelloCardsList {
	res, err := http.Get(url)
	if err != nil {
		panic(err.Error())
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err.Error())
	}
	s := new(TrelloCardsList)
	err = json.Unmarshal(body, &s)
	return s
}

func main() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Panic(err)
	}

	// bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
			fmt.Printf("\n--- (debug) casting new vote: ")
			msg := createUpdateResponse(update)
			bot.Send(msg)
		} else if update.Message != nil {
			doChat(*bot, update)
		}

	}
}
