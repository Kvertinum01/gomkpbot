package vkbot

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/Kvertinum01/gomkpbot/internal/app/vkapi"
)

type MessageEvent struct {
	ConservationMessageID int               `json:"conversation_message_id"`
	EventID               string            `json:"event_id"`
	Payload               map[string]string `json:"payload"`
	PeerID                int               `json:"peer_id"`
	UserID                int               `json:"user_id"`
}

func showSnackbar(text string) string {
	return fmt.Sprintf(
		"{\"type\": \"show_snackbar\", \"text\": \"%s\"}",
		text,
	)
}

func (bot *Bot) convertMap(m map[string]interface{}, target interface{}) error {
	jsonString, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonString, target)
}

func (bot *Bot) checkMessageEvent(m *MessageEvent) error {
	// Get duel type and duel id from payload
	duelType, ok := m.Payload["type"]
	if !ok {
		return nil
	}
	duelID, ok := m.Payload["duel_id"]
	if !ok {
		return nil
	}
	intDuelID, err := strconv.Atoi(duelID)
	if err != nil {
		return nil
	}

	// Find duel
	duel, ok := bot.duels[intDuelID]
	if !ok {
		return nil
	}

	// Check user id
	if duel.NowWay != m.UserID {
		return bot.sendAnswer(m, "Сейчас не ваш ход")
	}

	// Get way from payload
	way, ok := m.Payload["way"]
	if !ok {
		return nil
	}
	intWay, err := strconv.Atoi(way)
	if err != nil {
		return nil
	}

	switch duelType {
	case "attack":
		// Do attack
		duel.Members[m.UserID].Attack = intWay

		kjson, err := createDefendKeyboard(intDuelID)
		if err != nil {
			return err
		}

		model := duel.Members[m.UserID].Model

		answer := fmt.Sprintf(
			"[id%v|%s], вы атаковали. Что будете защищать:",
			m.UserID, model.UserName,
		)
		if err := bot.sendAnswer(m, "Вы походили!"); err != nil {
			return err
		}
		return bot.api.Method("messages.send", map[string]interface{}{
			"peer_id":   m.PeerID,
			"random_id": 0,
			"message":   answer,
			"keyboard":  kjson,
		}, nil)

	case "defend":
		// Do defend
		duel.Members[m.UserID].Protect = intWay
		duel.Ways += 1

		if duel.Ways == 2 {
			if err := bot.sendAnswer(m, "Дуэль закончилась!"); err != nil {
				return err
			}
			// Finish game
			firstMemberID := duel.NowWay
			secondMemberID := duel.AnotherMember
			firstMember := duel.Members[firstMemberID]
			secondMember := duel.Members[secondMemberID]

			if firstMember.Attack != secondMember.Protect {
				firstMember.IsWin = true
			}
			if firstMember.Protect != secondMember.Attack {
				secondMember.IsWin = true
			}
			if firstMember.IsWin && secondMember.IsWin {
				// Draw
				if err := bot.send(
					m.PeerID, "Игра закончилась. Ничья!",
				); err != nil {
					return err
				}
			} else if firstMember.IsWin {
				// Win first member
				if err := bot.store.User().WinByID(
					firstMemberID, secondMemberID,
				); err != nil {
					return err
				}
				answer := fmt.Sprintf(
					"Игра закончилась. Победил: [id%v|%s]!",
					firstMemberID, firstMember.Model.UserName,
				)
				if err := bot.send(m.PeerID, answer); err != nil {
					return err
				}
			} else if secondMember.IsWin {
				// Win second member
				if err := bot.store.User().WinByID(
					secondMemberID, firstMemberID,
				); err != nil {
					return err
				}
				answer := fmt.Sprintf(
					"Игра закончилась! Победил: [id%v|%s]!",
					secondMemberID, secondMember.Model.UserName,
				)
				if err := bot.send(m.PeerID, answer); err != nil {
					return err
				}
			}
			delete(bot.duels, intDuelID)
			return nil
		}

		duel.NowWay = duel.AnotherMember
		duel.AnotherMember = m.UserID
		model := duel.Members[duel.NowWay].Model

		kjson, err := createAttackKeyboard(intDuelID)
		if err != nil {
			log.Fatal(err)
		}
		answer := fmt.Sprintf(
			"Вы защитились, теперь атакует: [id%v|%s]",
			duel.NowWay, model.UserName,
		)
		if err := bot.sendAnswer(m, "Вы походили!"); err != nil {
			return err
		}
		return bot.api.Method("messages.send", map[string]interface{}{
			"peer_id":   m.PeerID,
			"random_id": 0,
			"message":   answer,
			"keyboard":  kjson,
		}, nil)
	}
	return nil
}

func (bot *Bot) sendAnswer(m *MessageEvent, event_data string) error {
	// Send message event answer
	return bot.api.Method("messages.sendMessageEventAnswer", map[string]interface{}{
		"event_id":   m.EventID,
		"user_id":    m.UserID,
		"peer_id":    m.PeerID,
		"event_data": showSnackbar(event_data),
	}, nil)
}

func createDefendKeyboard(duelID int) (string, error) {
	// Create defend keyboard
	parts := map[int]string{
		1: "Голова",
		2: "Живот",
		3: "Руки",
		4: "Ноги",
	}

	k := vkapi.NewKeyboard(false, true)
	for i := 1; i <= 4; i++ {
		k.Add(vkapi.NewCallbackButton(
			parts[i], fmt.Sprintf(
				"{\"way\": \"%v\", \"type\": \"defend\", \"duel_id\": \"%v\"}",
				i, duelID,
			), "positive",
		))
		if i == 2 {
			k.NewLine()
		}
	}
	k.NewLine()
	return k.GetJson()
}

func (bot *Bot) checkEvent(event vkapi.LongpollMessage) {
	// Check mesage event
	switch event.Type {
	case "message_event":
		m := &MessageEvent{}
		if err := bot.convertMap(event.Object, &m); err != nil {
			log.Fatal(err)
		}
		if err := bot.checkMessageEvent(m); err != nil {
			log.Fatal(err)
		}
	}
}