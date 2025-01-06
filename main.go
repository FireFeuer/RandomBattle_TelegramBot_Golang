package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Player struct {
	ID        int64
	Username  string
	Health    int  // Очки здоровья
	canAttack bool // Флаг для контроля очередности атак
}

var players = make(map[int64]*Player)
var mutex sync.Mutex

func main() {
	rand.Seed(time.Now().UnixNano()) // Инициализация генератора случайных чисел

	botToken := "" // Замените на ваш токен
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	file, err := os.OpenFile("log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	logger := log.New(file, "", log.LstdFlags|log.Lshortfile)

	u := tgbotapi.UpdateConfig{
		Offset:  0,
		Timeout: 60,
	}

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		msg := update.Message
		userID := msg.From.ID
		username := msg.From.UserName

		logger.Printf("Received message from %d (%s): %s", userID, username, msg.Text)

		switch strings.ToLower(msg.Text) {
		case "/start":
			startGame(bot, logger, userID, username)
		case "/list":
			listPlayers(bot, logger, userID)
		default:
			// Получаем шансы на атаки перед обработкой ввода
			attack1Damage, attack1Chance, attack2Damage, attack2Chance := sendAttackOptions(bot, logger, userID)
			handleInput(bot, logger, userID, username, msg.Text, attack1Damage, attack1Chance, attack2Damage, attack2Chance)
		}
	}
}

func startGame(bot *tgbotapi.BotAPI, logger *log.Logger, userID int64, username string) {
	mutex.Lock()
	defer mutex.Unlock()

	player := &Player{ID: userID, Username: username, Health: 100, canAttack: true}
	if _, exists := players[userID]; !exists {
		players[userID] = player
	}

	var messageText string
	if len(players) >= 2 {
		opponent := getOpponent(userID)

		messageText = fmt.Sprintf("Игра началась! Участники:\n%s (Здоровье: %d)\n%s (Здоровье: %d)",
			getUserInfo(players[userID]),
			player.Health,
			getUserInfo(opponent),
			opponent.Health)

		sendMessage(bot, logger, userID, messageText)
		sendMessage(bot, logger, opponent.ID, messageText)

		// Уведомляем обоих игроков о возможных атаках
		sendAttackOptions(bot, logger, userID)
		sendAttackOptions(bot, logger, opponent.ID)
	} else {
		messageText = "Ожидается другой игрок..."
		sendMessage(bot, logger, userID, messageText)
	}
}

func sendAttackOptions(bot *tgbotapi.BotAPI, logger *log.Logger, userID int64) (int, int, int, int) {
	var attack1Damage, attack1Chance, attack2Damage, attack2Chance int

	player := players[userID]
	if player == nil || !player.canAttack { // Игрок не подключен к игре или не может атаковать
		return 0, 0, 0, 0
	}

	for {
		// Генерация значений для обычной атаки
		attack1Damage = rand.Intn(100) + 1 // Урон для обычной атаки (1) от 1 до 100
		attack1Chance = rand.Intn(100) + 1 // Шанс успешной обычной атаки от 1 до 100

		// Генерация значений для специальной атаки
		attack2Damage = rand.Intn(100) + 1 // Урон для специальной атаки (2) от 1 до 100
		attack2Chance = rand.Intn(100) + 1 // Шанс успешной специальной атаки от 1 до 100

		// Проверяем условия: attack1Chance > attack2Chance и attack1Damage < attack2Damage
		if attack1Chance > attack2Chance && attack1Damage < attack2Damage {
			break // Условия выполнены, выходим из цикла
		}
	}

	message := fmt.Sprintf("Возможные атаки:\n1: Обычная атака (Урон: %d, Шанс: %d%%)\n2: Специальная атака (Урон: %d, Шанс: %d%%)\n\nВведите '1' для обычной атаки или '2' для специальной атаки:",
		attack1Damage, attack1Chance, attack2Damage, attack2Chance)

	sendMessage(bot, logger, userID, message)

	return attack1Damage, attack1Chance, attack2Damage, attack2Chance // Возвращаем урон и шансы
}

func handleInput(bot *tgbotapi.BotAPI, logger *log.Logger, userID int64, username string, input string, attack1Damage int, attack1Chance int, attack2Damage int, attack2Chance int) {
	mutex.Lock()
	defer mutex.Unlock()

	player := players[userID]
	if player == nil || !player.canAttack { // Игрок не подключен к игре или не может атаковать
		sendMessage(bot, logger,
			userID,
			"Сейчас не ваша очередь атаковать.")
		return
	}

	opponent := getOpponent(userID)
	if opponent == nil {
		return // Противник не найден
	}

	var damage int

	if input == "1" {
		chance := rand.Intn(100) + 1 // Генерируем случайный шанс от 1 до 100 для обычной атаки

		if chance <= attack1Chance { // Проверяем успешность атаки
			damage = attack1Damage // Используем заранее сгенерированный урон

			opponent.Health -= damage // Уменьшаем здоровье противника

			sendMessage(bot, logger,
				opponent.ID,
				fmt.Sprintf("Игрок @%s атакует с шансом %d%%! Вы потеряли %d здоровья. Текущее здоровье: %d",
					username, chance, damage, opponent.Health))

			sendMessage(bot, logger,
				userID,
				fmt.Sprintf("Вы атаковали @%s с шансом %d%%! Он потерял %d здоровья. Текущее здоровье противника: %d", opponent.Username, chance, damage, opponent.Health))
		} else {
			sendMessage(bot, logger,
				userID,
				fmt.Sprintf("Атака не удалась! Шанс был %d%%.", chance))
			return
		}
	} else if input == "2" {
		chance := rand.Intn(100) + 1 // Генерируем случайный шанс от 1 до 100 для специальной атаки

		if chance <= attack2Chance { // Проверяем успешность специальной атаки
			damage = attack2Damage // Используем заранее сгенерированный урон

			opponent.Health -= damage

			sendMessage(bot, logger,
				opponent.ID,
				fmt.Sprintf("Игрок @%s использует специальную атаку с шансом %d%%! Вы потеряли %d здоровья. Текущее здоровье: %d",
					username, chance, damage, opponent.Health))

			sendMessage(bot, logger,
				userID,
				fmt.Sprintf("Вы использовали специальную атаку против @%s с шансом %d%%! Он потерял %d здоровья. Текущее здоровье противника: %d", opponent.Username, chance, damage, opponent.Health))
		} else {
			sendMessage(bot, logger,
				userID,
				fmt.Sprintf("Специальная атака не удалась! Шанс был %d%%.", chance))
			return
		}
	} else {
		sendMessage(bot, logger,
			userID,
			"Пожалуйста введите '1' для обычной атаки или '2' для специальной атаки.")
		return
	}

	if opponent.Health <= 0 {
		sendMessage(bot, logger,
			userID,
			fmt.Sprintf("Вы победили! @%s больше не может сражаться.", opponent.Username))
		sendMessage(bot, logger,
			opponent.ID,
			fmt.Sprintf("Вы проиграли! @%s одержал победу.", username))

		delete(players, userID)
		delete(players, opponent.ID)
		return
	}

	player.canAttack = false
	opponent.canAttack = true

	sendMessage(bot, logger,
		opponent.ID,
		fmt.Sprintf("Теперь ваш ход! Текущее здоровье @%s: %d.",
			player.Username, player.Health))

	sendMessage(bot, logger,
		userID,
		fmt.Sprintf("Текущее здоровье @%s: %d.", opponent.Username, opponent.Health))
}

func listPlayers(bot *tgbotapi.BotAPI, logger *log.Logger, userID int64) {
	mutex.Lock()
	defer mutex.Unlock()

	var messageText string
	if len(players) == 0 {
		messageText = "Сейчас нет активных игроков."
	} else {
		messageText = "Подключённые игроки:\n"
		for _, player := range players {
			messageText += fmt.Sprintf("- @%s (%d)\n", player.Username, player.ID)
		}
	}

	sendMessage(bot, logger, userID, messageText)
}

func getOpponent(userID int64) *Player {
	for _, p := range players {
		if p.ID != userID {
			return p
		}
	}
	return nil
}

func getUserInfo(player *Player) string {
	return fmt.Sprintf("@%s (%d)", player.Username, player.ID)
}

func sendMessage(bot *tgbotapi.BotAPI, logger *log.Logger, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := bot.Send(msg); err != nil {
		logger.Printf("Error sending message to %d: %v", chatID, err)
	}
}
