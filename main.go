package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"gopkg.in/yaml.v2"
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
	Name   string         `yaml:name`
	ID     string         `yaml:id`
	Votes  []Proposal     `yaml:proposal`
	Voters map[string]int `yaml:voters`
}

type Proposal struct {
	Vote        int    `yaml:vote`
	Description string `yaml:description`
}

func createYAMLFormData(cardList *TrelloCardsList) string {
	election := new(Election)
	election.Name = cardList.Name
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

func chat(update tgbotapi.Update) tgbotapi.MessageConfig {

	msgTxt := update.Message.Text
	msgParts := strings.Split(msgTxt, " ")
	fmt.Println("\n--- (debug) /newvote: ", msgParts)
	fmt.Println(len(msgParts), msgParts)
	if msgTxt == "/close" {
		if update.Message.ReplyToMessage == nil {
			return tgbotapi.NewMessage(update.Message.Chat.ID, "Tu dois repondre au poll que tu souhaites fermer")
		} else {
			fmt.Println(update.Message.ReplyToMessage.MessageID)
		}
	}
	if len(msgParts) == 2 && msgParts[0] == "/newvote" {

		voteUrl, err := url.Parse(msgParts[1])
		if err != nil {
			return tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid URL, please try again.")
		}
		if voteUrl.Scheme == "" {
			voteUrl.Scheme = "https"
		}
		if voteUrl.Host == "" {
			return tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid URL host, please try again.")
		}
		trelloCardsList := getTrelloCards(voteUrl.String())

		votingText := createYAMLFormData(trelloCardsList)

		buttonMarkup := createButtonForm(trelloCardsList)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, votingText)
		msg.ReplyMarkup = buttonMarkup

		msg.ReplyToMessageID = update.Message.MessageID
		fmt.Printf("\n--- (debug) sending the form\n ")
		return msg
	} else {
		return tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command, please try again.")
	}
}

func createUpdateResponse(update tgbotapi.Update) tgbotapi.EditMessageTextConfig {
	choice := strings.TrimLeft(update.CallbackQuery.Data, "/")
	voteID, err := strconv.Atoi(choice)
	fmt.Printf("\n--- (debug)  ...newvote cast for %s: %s", update.CallbackQuery.From.UserName, choice)
	if err != nil {
		fmt.Println(err)
		os.Exit(127)
	}
	votingForm := Election{}
	err = yaml.Unmarshal([]byte(update.CallbackQuery.Message.Text), &votingForm)
	if err != nil {
		fmt.Println(err)
		os.Exit(127)
	}

	votingForm.Votes[voteID].Vote += 1
	if SINGLE_VOTE {
		previousVoteChoice, ok := votingForm.Voters[update.CallbackQuery.From.UserName]
		if ok {
			votingForm.Votes[previousVoteChoice].Vote -= 1
		}
	}
	votingForm.Voters[update.CallbackQuery.From.UserName] = voteID
	proposals := []string{}
	for _, proposal := range votingForm.Votes {
		proposals = append(proposals, proposal.Description)
	}
	newPropositions, err := yaml.Marshal(&votingForm)
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
			msg := chat(update)
			bot.Send(msg)
		}

	}
}
