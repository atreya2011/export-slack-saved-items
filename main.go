package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/slack-go/slack"
	"gopkg.in/yaml.v3"
)

type config struct {
	Token string `yaml:"token"`
}

type starredItemForCSV struct {
	itemNo      string
	timeStamp   string
	userName    string
	description string
}

func (c *config) parse() string {
	// load the yaml file
	yamlFile, err := ioutil.ReadFile("token.yml")
	if err != nil {
		log.Printf("yamlFile.Get error #%v\n", err)
	}
	// parse the yaml file
	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		log.Fatalf("Unmarshal: %v\n", err)
	}
	// return the token
	return c.Token
}

func main() {
	// initialize flags
	var debug, getStarred, delMessages bool
	flag.BoolVar(&debug, "debug", false, "Show JSON output")
	flag.BoolVar(&getStarred, "get-starred", false, "get starred")
	flag.BoolVar(&delMessages, "del-msg", false, "delete messages")
	flag.Parse()
	// load and parse config
	c := new(config)
	c.parse()
	// initialize slack client
	api := slack.New(c.Token, slack.OptionDebug(debug))
	if getStarred {
		getStarredItems(api, debug)
	}
	if delMessages {
		deleteMessages(api)
	}
}

func deleteMessages(api *slack.Client) {
	res, err := api.GetConversationHistory(&slack.GetConversationHistoryParameters{
		ChannelID: "DDAPBL3M1",
	})
	if err != nil {
		log.Println(err)
	}
	// nextCursor := res.ResponseMetaData.NextCursor
	messages := res.Messages
	// for nextCursor != "" {
	// log.Println(nextCursor)
	for i, msg := range messages {
		_, _, err := api.DeleteMessage("DDAPBL3M1", msg.Timestamp)
		if err != nil {
			log.Println(err)
		}
		log.Println(i)
	}
	// res, err := api.GetConversationHistory(&slack.GetConversationHistoryParameters{
	// 	ChannelID: "DDAPBL3M1",
	// 	Cursor:    nextCursor,
	// })
	// if err != nil {
	// 	log.Println(err)
	// }
	// nextCursor = res.ResponseMetaData.NextCursor
	// messages = res.Messages
	// }
}

func getStarredItems(api *slack.Client, debug bool) {
	// get starred items for creating csv
	starredItems := getStarred(api, debug)
	// create empty csv file
	f, err := os.Create("starred.csv")
	if err != nil {
		fmt.Println(err)
		f.Close()
		return
	}
	// convert starred items to [][]string and store in data
	var data [][]string
	for _, si := range starredItems {
		data = append(data, []string{si.itemNo, si.timeStamp, si.userName, si.description})
	}
	w := csv.NewWriter(f)
	// write the data to csv
	// WriteAll calls Flush internally
	w.WriteAll(data)
	if err := w.Error(); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("file written successfully")
}

func getStarred(api *slack.Client, debug bool) (starredItemsForCSV []starredItemForCSV) {
	// create markdown file for storing data for debugging
	var g *os.File
	if debug {
		g, err := os.Create("item-dump.md")
		if err != nil {
			fmt.Println(err)
			g.Close()
			return starredItemsForCSV
		}
	}
	// Get all stars for the user and handle errors
	starredItems, err := api.ListAllStars()
	if err != nil {
		fmt.Printf("Error getting stars: %s\n", err)
		return starredItemsForCSV
	}
	// Inform user the total count of starred items
	fmt.Println(len(starredItems))
	count := make(chan int) // channel for measuring progress
	done := make(chan bool) // channel for informing if looping is finished
	go func() {
		// loop in reverse order as stars are returned in reverse chronological order
		for i := len(starredItems) - 1; i >= 0; i-- {
			// initialize variables to be stored in starredItemForCSV struct
			var (
				desc      string
				user      *slack.User
				userName  string
				timeStamp time.Time
			)
			// check the type of starred item
			switch starredItems[i].Type {
			case slack.TYPE_MESSAGE:
				desc = starredItems[i].Message.Text
				if desc == "" && len(starredItems[i].Message.Files) == 0 {
					for _, a := range starredItems[i].Message.Attachments {
						desc += fmt.Sprintf("%s\n", a.Text)
					}
				}
				if desc == "" && len(starredItems[i].Message.Files) > 0 {
					for _, file := range starredItems[i].Message.Files {
						desc += fmt.Sprintf("%s\n", file.URLPrivate)
					}
				}
				user, err = api.GetUserInfo(starredItems[i].Message.User)
				if err == nil {
					userName = user.Name
				}
				tsInt, _ := strconv.ParseFloat(starredItems[i].Message.Timestamp, 64)
				seconds, _ := math.Modf(tsInt)
				timeStamp = time.Unix(int64(seconds), 0)
			case slack.TYPE_FILE:
				desc = starredItems[i].File.URLPrivateDownload
			case slack.TYPE_FILE_COMMENT:
				desc = starredItems[i].File.Name + " - " + starredItems[i].Comment.Comment
			case slack.TYPE_CHANNEL, slack.TYPE_IM, slack.TYPE_GROUP:
				desc = starredItems[i].Channel
			}
			count <- i
			starredItemsForCSV = append(starredItemsForCSV,
				starredItemForCSV{
					itemNo:      strconv.Itoa(i),
					timeStamp:   timeStamp.Format("2006-01-02 15:04:05"),
					userName:    userName,
					description: desc,
				})
			if debug {
				fmt.Fprintf(g, "%03d\n%s\n", i, "```")
				spew.Fdump(g, starredItems[i])
				fmt.Fprintf(g, "%s\n", "```")
			}
		}
		done <- true
	}()

	for {
		select {
		case count := <-count:
			fmt.Printf("\r%d percent", count*(100/len(starredItems)))
		case <-done:
			if debug {
				err = g.Close()
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println("file written successfully")
			}
			return starredItemsForCSV
		}
	}
}
