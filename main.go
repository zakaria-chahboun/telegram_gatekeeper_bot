package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	tb "gopkg.in/telebot.v3"
)

// Constants
const (
	MATH_QUIZ_TIMEOUT  = 15 // Timeout for math quiz in seconds
	CLEAN_CHAT_TIMEOUT = 20 // Timeout for cleaning chat messages in seconds
)

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(".env"); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// Get the bot token from the environment variables
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN is not set in the environment variables")
	}

	// Initialize the bot
	bot, err := tb.NewBot(tb.Settings{
		Token:  botToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Handle chat join requests
	bot.Handle(tb.OnChatJoinRequest, func(c tb.Context) error {
		return handleChatJoinRequest(bot, c)
	})

	// Handle /start command
	bot.Handle("/start", func(c tb.Context) error {
		return handleStartCommand(bot, c)
	})

	// Handle callback queries
	bot.Handle(tb.OnCallback, func(c tb.Context) error {
		return handleCallback(bot, c)
	})

	// Start the bot
	fmt.Println("Telegram Bot, Start listening ...")
	bot.Start()
}

// handleChatJoinRequest handles the event when a user requests to join the group.
func handleChatJoinRequest(bot *tb.Bot, c tb.Context) error {
	user := c.Sender()
	chat := c.Chat()

	if isSubscribed(bot, chat, user) {
		return handleValidationProcess(bot, chat, user)
	}
	return sendSubscriptionMessage(bot, chat, user)
}

// isSubscribed checks if the user is subscribed to the bot
func isSubscribed(bot *tb.Bot, chat *tb.Chat, user *tb.User) bool {
	member, err := bot.ChatMemberOf(chat, user)
	if err != nil {
		log.Printf("Error checking subscription: %v", err)
		return false
	}
	return member != nil
}

// sendSubscriptionMessage sends a message with an inline button to redirect the user to the bot for subscription
func sendSubscriptionMessage(bot *tb.Bot, chat *tb.Chat, user *tb.User) error {
	startMessage := "لإكمال عملية التحقق، يرجى الاشتراك في البوت عبر الضغط على الزر أدناه ثم العودة لإكمال التحقق."

	// Create a new markup for inline buttons
	markup := bot.NewMarkup()

	// Create the inline button
	btn := markup.URL("ابدأ التحقق", "https://t.me/"+bot.Me.Username)

	// Add the button to a row
	markup.Inline(
		markup.Row(btn),
	)

	// Send the message with the inline button
	msg, err := bot.Send(chat, startMessage, markup)
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}

	// Delete the message after CLEAN_CHAT_TIMEOUT seconds
	time.AfterFunc(CLEAN_CHAT_TIMEOUT*time.Second, func() {
		if err := bot.Delete(msg); err != nil {
			log.Printf("Error deleting message: %v", err)
		}
	})

	return nil
}

// handleValidationProcess starts the validation process for the user
func handleValidationProcess(bot *tb.Bot, chat *tb.Chat, user *tb.User) error {
	if _, err := bot.Send(user, "لإكمال عملية التحقق، يرجى حل المسألة التالية خلال 15 ثانية."); err != nil {
		return fmt.Errorf("error sending validation message: %w", err)
	}
	if correct := askMathProblem(bot, user); correct {
		if _, err := bot.Send(user, "إجابة صحيحة! يمكنك الآن الانضمام إلى المجموعة."); err != nil {
			return fmt.Errorf("error sending correct answer message: %w", err)
		}
		bot.ApproveJoinRequest(chat, user)
		return welcomeUserToGroup(bot, chat, user)
	}
	if _, err := bot.Send(user, "إجابة غير صحيحة! سيتم رفض طلبك للانضمام."); err != nil {
		return fmt.Errorf("error sending incorrect answer message: %w", err)
	}
	bot.DeclineJoinRequest(chat, user)
	return nil
}

// askMathProblem asks the user to solve a math problem and returns true if they solve it correctly.
func askMathProblem(bot *tb.Bot, user *tb.User) bool {
	// Generate a random math problem
	num1, num2 := rand.Intn(10), rand.Intn(10)
	correctAnswer := num1 + num2
	problem := fmt.Sprintf("يرجى حل هذه المسألة: %d + %d = ?", num1, num2)

	// Send the math problem first
	msg, err := bot.Send(user, problem)
	if err != nil {
		log.Printf("Error sending message: %v", err)
		return false
	}

	// Create a channel to receive the user's math answer
	mathAnswerChan := make(chan string)

	// Listen for the user's response
	go func() {
		bot.Handle(tb.OnText, func(c tb.Context) error {
			mathAnswerChan <- c.Message().Text
			return nil
		})
	}()

	// Update the countdown message every second
	go func() {
		for i := MATH_QUIZ_TIMEOUT; i > 0; i-- {
			time.Sleep(1 * time.Second)
			if _, err := bot.Edit(msg, fmt.Sprintf("يرجى حل هذه المسألة: %d + %d = ?\nالوقت المتبقي: %d ثانية", num1, num2, i)); err != nil {
				log.Printf("Error editing message: %v", err)
				return
			}
		}
	}()

	// Wait for the user's math answer or timeout after MATH_QUIZ_TIMEOUT seconds
	select {
	case mathAnswer := <-mathAnswerChan:
		if userMathAnswer, err := strconv.Atoi(mathAnswer); err == nil && userMathAnswer == correctAnswer {
			return true
		}
		return false
	case <-time.After(MATH_QUIZ_TIMEOUT * time.Second):
		if _, err := bot.Send(user, "انتهى الوقت!."); err != nil {
			log.Printf("Error sending timeout message: %v", err)
		}
		return false
	}
}

// welcomeUserToGroup sends a welcome message to the group.
func welcomeUserToGroup(bot *tb.Bot, chat *tb.Chat, user *tb.User) error {
	welcomeMessage := fmt.Sprintf("سادتي وسيداتي رحبوا معنا بالوافد الجديد %s! لقد تم قبوله معنا 🤠🎉", user.FirstName)
	if _, err := bot.Send(chat, welcomeMessage); err != nil {
		return fmt.Errorf("error sending welcome message: %w", err)
	}
	return nil
}

// handleStartCommand handles the /start command and sends a welcome message.
func handleStartCommand(bot *tb.Bot, c tb.Context) error {
	startMessage := "مرحباً! أنا البواب الحارس، الغوفر 🍉، أساعدكم على طرد الوافدين الجدد المخادعين."
	if _, err := bot.Send(c.Chat(), startMessage); err != nil {
		return fmt.Errorf("error sending start message: %w", err)
	}
	return nil
}

// handleCallback handles the callback query for starting the validation process
func handleCallback(bot *tb.Bot, c tb.Context) error {
	callback := c.Callback()
	if callback.Data == "start_validation" {
		user := callback.Sender
		if _, err := bot.Send(user, "مرحبًا! للتحقق، يرجى حل المسألة التالية."); err != nil {
			return fmt.Errorf("error sending validation message: %w", err)
		}
		if correct := askMathProblem(bot, user); correct {
			if _, err := bot.Send(user, "إجابة صحيحة! يمكنك الآن الانضمام إلى المجموعة."); err != nil {
				return fmt.Errorf("error sending correct answer message: %w", err)
			}
		} else {
			if _, err := bot.Send(user, "إجابة غير صحيحة! سيتم رفض طلبك للانضمام."); err != nil {
				return fmt.Errorf("error sending incorrect answer message: %w", err)
			}
		}
		return bot.Respond(callback, &tb.CallbackResponse{Text: "تم إرسال رسالة خاصة للتحقق."})
	}
	return nil
}
