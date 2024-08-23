package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	tb "gopkg.in/telebot.v3"
)

// Global constants for timeouts
const (
	MATH_TIMEOUT          = 15
	CLEAN_MESSAGE_TIMEOUT = 20
)

func main() {
	// Load environment variables from .env file
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("loading .env file has failed:", err)
	}

	// Get the bot token from the environment variables
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN is not set in the environment variables")
	}

	pref := tb.Settings{
		Token:  botToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := tb.NewBot(pref)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Handle the event when a user joins the group
	bot.Handle(tb.OnUserJoined, func(c tb.Context) error {

		// logs
		log.Println(c.Sender().Username, "joining ..")

		handleUserJoin(bot, c)
		return nil
	})

	// Handle the /start command for verification
	bot.Handle("/start", func(c tb.Context) error {
		args := c.Message().Payload

		// logs
		log.Println(c.Sender().Username, ": /start", args)

		if args != "" {
			handleStartCommand(bot, c, args)
		} else {
			bot.Send(c.Chat(), "/start GROUP_USER_NAME")
		}
		return nil
	})

	// Start the bot
	fmt.Println("Telegram Bot, Start listening ...")
	bot.Start()
}

// handleUserJoin handles the event when a user joins the group and sends a verification message.
func handleUserJoin(bot *tb.Bot, c tb.Context) {
	chat := c.Chat()
	user := c.Sender()

	// Restrict the user by default
	err := bot.Restrict(chat, &tb.ChatMember{
		User: user,
		Rights: tb.Rights{
			CanSendMessages: false,
			CanSendMedia:    false,
			CanSendPolls:    false,
			CanPostMessages: false,
			CanSendOther:    false,
			CanAddPreviews:  false,
		},
	})
	if err != nil {
		log.Println("Error restricting user:", err)
		return
	}

	// Create the inline button for verification
	markup := bot.NewMarkup()
	btn := markup.URL("ابدأ التحقق", "https://t.me/"+bot.Me.Username+"?start="+chat.Username)
	markup.Inline(markup.Row(btn))

	// Send the message with the inline button to the group
	startMessage := fmt.Sprintf("مرحبًا %s! للتحقق، يرجى الضغط على الزر أدناه.", user.FirstName)
	msg, err := bot.Send(chat, startMessage, markup)
	if err != nil {
		log.Println("Error sending message:", err)
		return
	}

	// Delete the message with the button after 20 seconds
	time.AfterFunc(CLEAN_MESSAGE_TIMEOUT*time.Second, func() {
		// logs
		log.Println(c.Sender().Username, ": Clean up for")
		bot.Delete(msg)
	})
}

// handleStartCommand handles the /start command and initiates the verification process.
func handleStartCommand(bot *tb.Bot, c tb.Context, groupUsername string) {
	user := c.Sender()

	// check the @ prefix
	if !strings.HasPrefix(groupUsername, "@") {
		groupUsername = "@" + groupUsername
	}

	groupChat, err := bot.ChatByUsername(groupUsername)
	if err != nil {
		bot.Send(user, fmt.Sprintf("عذرا، هذه المجموعة %s ليست ضمن قائمتي.", groupUsername))
		return
	}

	// Check if the bot has the necessary privileges in the group
	member, err := bot.ChatMemberOf(groupChat, bot.Me)
	if err != nil || !member.CanRestrictMembers || !member.CanInviteUsers || !member.CanDeleteMessages {
		//fmt.Printf("Rights: %+v", member.Rights)
		bot.Send(user, fmt.Sprintf("عذرا، ليست لدي الصلاحيات الكافية في هذه المجموعة %s", groupUsername))
		return
	}

	// Start the verification process
	verificationMessage := fmt.Sprintf("جار التحقق من دخولك إلى مجموعة 🔍:\n[%s](https://t.me/%s)", groupChat.Title, groupChat.Username)
	bot.Send(user, verificationMessage, &tb.SendOptions{ParseMode: tb.ModeMarkdownV2, DisableWebPagePreview: true})
	bot.Send(user, fmt.Sprintf("يرجى حل المسألة التالية خلال %v ثانية.", MATH_TIMEOUT))
	// give user O2
	time.Sleep(1 * time.Second)
	// start the quiz
	if askMathProblem(bot, user) {
		// Create the inline button for verification
		markup := bot.NewMarkup()
		btn := markup.URL("الرجوع للمجموعة", "https://t.me/"+groupChat.Username)
		markup.Inline(markup.Row(btn))
		bot.Send(user, "إجابة صحيحة! يمكنك الآن الانضمام 🎉", markup)

		// Remove restrictions
		err = bot.Promote(groupChat, &tb.ChatMember{
			User: user,
			Rights: tb.Rights{
				CanSendMessages: true,
				CanSendMedia:    true,
				CanAddPreviews:  false,
			},
		})
		if err != nil {
			log.Println("Error removing restrictions:", err)
		}
		welcomeUserToGroup(bot, groupChat, user)
	} else {
		bot.Send(user, "إجابة غير صحيحة! طلبك للانضمام مرفوض.")
		// Kick the user without banning
		err = bot.Unban(groupChat, user) // The third parameter defaults to false
		if err != nil {
			log.Println("Error kicking user:", err)
		}
	}
}

// askMathProblem asks the user to solve a math problem and returns true if they solve it correctly.
func askMathProblem(bot *tb.Bot, user *tb.User) bool {
	// Generate a random math problem
	num1 := rand.Intn(10)
	num2 := rand.Intn(10)
	correctAnswer := num1 + num2
	problem := fmt.Sprintf("%d + %d = ?", num1, num2)

	// Send the initial problem message
	msg, err := bot.Send(user, problem)
	if err != nil {
		log.Println("Error sending message:", err)
		return false
	}

	// Create a channel to receive the user's math answer
	mathAnswerChan := make(chan string)
	stopCountdownChan := make(chan struct{})

	// Listen for the user's response
	go func() {
		bot.Handle(tb.OnText, func(c tb.Context) error {
			mathAnswerChan <- c.Message().Text
			close(stopCountdownChan) // Stop the countdown
			return nil
		})
	}()

	// Update the countdown message every second
	go func() {
		for i := MATH_TIMEOUT; i >= 0; i-- {
			select {
			case <-stopCountdownChan:
				return // Stop updating the countdown
			case <-time.After(1 * time.Second):
				if _, err := bot.Edit(msg, fmt.Sprintf(problem+"\n"+"الوقت المتبقي: %d ثانية", i)); err != nil {
					log.Println("Error editing message:", err)
					return
				}
			}
		}
	}()

	// Wait for the user's math answer or timeout after MATH_TIMEOUT seconds
	select {
	case mathAnswer := <-mathAnswerChan:
		if userMathAnswer, err := strconv.Atoi(mathAnswer); err == nil && userMathAnswer == correctAnswer {
			return true
		}
		return false
	case <-time.After((MATH_TIMEOUT + 4) * time.Second):
		bot.Send(user, "انتهت المُهلة! طلبك للانضمام مرفوض.")
		return false
	}
}

// welcomeUserToGroup sends a welcome message to the group.
func welcomeUserToGroup(bot *tb.Bot, chat *tb.Chat, user *tb.User) {
	welcomeMessage := fmt.Sprintf("أهلا بك معنا @%s 🍉🎉", user.Username)
	_, err := bot.Send(chat, welcomeMessage)
	if err != nil {
		log.Println("Error sending welcoming markdown: ", err)
	}
}
