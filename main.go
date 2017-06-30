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
	"sort"
	"strconv"
	"strings"
)

const (
	SINGLE_VOTE = "single"
	MULTIPLE_VOTE_HONEST = "multiple"
	MULTIPLE_VOTE_WAR = "war"
)
	
var (
	SECRET_VOTE = true
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
	mode       string     `yaml:mode`
	PollsterID int        `yaml:pollster`
	Votes      []Proposal `yaml:proposal`
	// The string is a secret function of the username, so as to protect voter identity
	Voters map[string]int `yaml:voters`
}

type Proposal struct {
	Vote        int    `yaml:vote`
	Description string `yaml:description`
}

func (election Election) Len() int {
	return len(election.Votes)
}

func (election Election) Less(i, j int) bool {
	return election.Votes[i].Vote < election.Votes[j].Vote
}

func (election Election) Swap(i, j int) {
	election.Votes[i], election.Votes[j] = election.Votes[j], election.Votes[i]
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
	election.mode = MULTIPLE_VOTE_HONEST
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

	if msgTxt == "/start" || msgTxt == "/start@"+bot.Self.UserName {
		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, getBotStartMsg(bot.Self.UserName)))
	} else if msgTxt == "/help" || msgTxt == "/help@"+bot.Self.UserName {
		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, getBotStartMsg(bot.Self.UserName)))
	} else if msgTxt == "/close" || msgTxt == "/close@"+bot.Self.UserName {
		if update.Message.ReplyToMessage == nil {
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Please use this command while responding to the poll you want to close."))
		} else {
			fmt.Printf("\n--- (debug) ...closing poll ID ? %d", update.Message.ReplyToMessage.MessageID)
			election := ElectionFromMessage(*update.Message.ReplyToMessage)
			// check identity of who is closing the poll - should be the same one who opened it
			if election.PollsterID != update.Message.From.ID {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "You have no right to close that poll.")
				msg.ReplyToMessageID = update.Message.MessageID
				bot.Send(msg)
				return
			}
			msg := closePoll(update.Message.ReplyToMessage)
			bot.Send(msg)
			summaryMsg := createPollSummary(*update.Message.ReplyToMessage, *update.Message.From)
			bot.Send(summaryMsg)

		}
	} else if len(msgParts) < 2 && (msgParts[0] == "/newvote" || msgParts[0] == "/newvote"+bot.Self.UserName) {
		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "/newvote command take an URL as argument, see /help"))
	} else if len(msgParts) == 2 && (msgParts[0] == "/newvote" || msgParts[0] == "/newvote"+bot.Self.UserName) {
		voteUrl, err := url.Parse(msgParts[1])
		if err != nil {
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid URL, please see /help"))
		}
		if voteUrl.Scheme == "" {
			voteUrl.Scheme = "https"
		}
		if voteUrl.Host == "" {
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid URL host, please see /help"))
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
		return fmt.Sprintf("%d", voter.ID)
	}
}

func createPollSummary(message tgbotapi.Message, pollster tgbotapi.User) tgbotapi.MessageConfig {
	election := ElectionFromMessage(message)
	var bufferSummary bytes.Buffer
	bufferSummary.WriteString(fmt.Sprintf("— Closing poll \"%s\" —\n", election.Name))
	fmt.Printf("%v\n", message.ReplyToMessage)
	if pollster.FirstName == "" && pollster.LastName == "" {
		bufferSummary.WriteString(fmt.Sprintf("Poll started by %v %v\n", pollster.FirstName, pollster.LastName))
	} else {
		bufferSummary.WriteString(fmt.Sprintf("Poll started by %v\n", pollster.UserName))
	}
	sort.Sort(sort.Reverse(election))
	for _, vote := range election.Votes {
		var voteNoun string
		if vote.Vote == 0 {
			break
		}
		if vote.Vote <= 1 {
			voteNoun = "vote"
		} else {
			voteNoun = "votes"
		}
		bufferSummary.WriteString(fmt.Sprintf(" ❧ %2d %s for «%s»\n", vote.Vote, voteNoun, vote.Description))
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

	currentBallot := election.Votes[voteID]
	currentBallot.Vote += 1
	previousVoteChoice, voteChoiceExists := election.Voters[voterID(update.CallbackQuery.From, update.Message)]
	if election.mode == SINGLE_VOTE {
		if voteChoiceExists {
			election.Votes[previousVoteChoice].Vote -= 1
		}
	} else if election.mode == MULTIPLE_VOTE_HONEST {
		// one can vote once for each option
		// TODO
	} else if election.mode == MULTIPLE_VOTE_WAR {
		// war mode
		
	} else {
		// mode not defined
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

func getBotStartMsg(botName string) string {

	botMsg := `Welcome to @` + botName + `!

This message may not be up to date, please visit
https://github.com/epfl-dojo/votbot for latest and
more complete documentation.

Please note that this bot is stateless; all vote data
are stored inline into the vote message.

The commands you may want to use are:
  ◦ /help
    Where you can have some tips and working demo.
  ◦ /newvote URL
    Where the URL point to a JSON file with specific elements.
    Public Trello card list URL can be used.
    Please check the README on github for all the jam recipe.
  ◦ /close
    When answering a poll message to close it;
    Vote options will disappear and a summarized message with
    vote's results will pop.

[WIP] Poll options can be defined in main.go:
  ◦ SINGLE_VOTE [true / false]
    That mean that every voter can check only one answer.
  ◦ SECRET_VOTE [true / false]
    That mean that voters username are not displayed.
  ◦ 💡 FOREVER_VOTE [true / false]
    That mean that every voter can check only one answer, once.

You can use this command: "/newvote https://raw.githubusercontent.com/epfl-dojo/votbot/master/minimal.json" as a demo...

Feel free to contact us, ask stuff or open issues
on https://github.com/epfl-dojo/votbot.

          — Have fun`
	return botMsg
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
