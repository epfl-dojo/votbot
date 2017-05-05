package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var (
	CARDS_JSON_URL   = "https://api.trello.com/1/lists/58c1b0c5a2c599346fd571ac?fields=name&cards=open&card_fields=name,url"
	RESULT_SEPARATOR = " - "
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
	Name  string     `yaml:name`
	Votes []Proposal `yaml:proposal`
}

type Proposal struct {
	Voters      []string `yaml:voters`
	Vote        int      `yaml:vote`
	Description string   `yaml:description`
}

func createYAMLFormData(cardList *TrelloCardsList) string {
	election := new(Election)
	for _, card := range cardList.Cards {
		proposal := Proposal{
			Description: card.Name,
			Vote:        0,
			Voters:      []string{},
		}
		election.Votes = append(election.Votes, proposal)
	}
	d, err := yaml.Marshal(&election)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Printf("--- t dump:\n%s\n\n", string(d))
	return string(d)
}

func createVoteForm(cardList *TrelloCardsList) string {
	formString := ""
	for _, card := range cardList.Cards {
		formString += "0" + RESULT_SEPARATOR + card.Name + "\n"
	}
	return formString
}

func createVoteResponse(update tgbotapi.Update) tgbotapi.MessageConfig {
	trelloCardsList := getTrelloCards(CARDS_JSON_URL)

	yaml := createYAMLFormData(trelloCardsList)
	fmt.Sprintf("%v", yaml)

	text := createVoteForm(trelloCardsList)
	buttonMarkup := createButtonForm(trelloCardsList)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
	msg.ReplyMarkup = buttonMarkup

	msg.ReplyToMessageID = update.Message.MessageID
	fmt.Printf("sending the form")
	return msg
}

func createUpdateResponse(update tgbotapi.Update) tgbotapi.EditMessageTextConfig {
	choice := strings.TrimLeft(update.CallbackQuery.Data, "/")
	voteID, err := strconv.Atoi(choice)
	fmt.Printf(choice)
	if err != nil {
		fmt.Println(err)
		os.Exit(127)
	}
	fmt.Println("vote choice is %v\n", voteID)
	votes := strings.Split(update.CallbackQuery.Message.Text, "\n")
	fmt.Println("splited votes len %v\n", len(votes))

	splitVote := strings.SplitN(votes[voteID], RESULT_SEPARATOR, 2)
	rawVoteCount := splitVote[0]
	voteCount, err := strconv.Atoi(rawVoteCount)
	if err != nil {
		fmt.Println(err)
		os.Exit(127)
	}
	votes[voteID] = fmt.Sprintf("%v"+RESULT_SEPARATOR+"%v", voteCount+1, strings.TrimSpace(splitVote[1]))
	newPropositions := strings.Join(votes, "\n")

	proposals := []string{}
	for _, proposal := range votes {
		splitVote := strings.SplitN(proposal, RESULT_SEPARATOR, 2)
		proposals = append(proposals, splitVote[1])
	}

	editMessageTextConfig := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, newPropositions)
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
			fmt.Printf("casting new vote\n")
			msg := createUpdateResponse(update)
			bot.Send(msg)
		} else if update.Message != nil {
			msg := createVoteResponse(update)
			bot.Send(msg)
		}

	}
}
