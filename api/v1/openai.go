package v1

import (
	"adams549659584/go-proxy-bingai/api"
	"adams549659584/go-proxy-bingai/common"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	binglib "github.com/Harry-zklcdc/bing-lib"
	"github.com/Harry-zklcdc/bing-lib/lib/hex"
	"github.com/Harry-zklcdc/bing-lib/lib/request"
)

var (
	apikey = os.Getenv("APIKEY")

	globalChat  = binglib.NewChat("")
	globalImage = binglib.NewImage("")
)

var STOPFLAG = "stop"

func ChatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if apikey != "" {
		if r.Header.Get("Authorization") != "Bearer "+apikey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	chat := globalChat.Clone()

	cookie := r.Header.Get("Cookie")
	if cookie == "" {
		if len(common.USER_TOKEN_LIST) > 0 {
			seed := time.Now().UnixNano()
			rng := rand.New(rand.NewSource(seed))
			cookie = common.USER_TOKEN_LIST[rng.Intn(len(common.USER_TOKEN_LIST))]
			chat.SetCookies(cookie)
		} else {
			cookie = chat.GetCookies()
		}
	}
	chat.SetCookies(cookie)

	resqB, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	var resq chatRequest
	json.Unmarshal(resqB, &resq)

	if resq.Model != binglib.BALANCED && resq.Model != binglib.BALANCED_OFFLINE && resq.Model != binglib.CREATIVE && resq.Model != binglib.CREATIVE_OFFLINE && resq.Model != binglib.PRECISE && resq.Model != binglib.PRECISE_OFFLINE {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if common.BingBaseUrl != "" {
		chat.SetBingBaseUrl(strings.ReplaceAll(strings.ReplaceAll(common.BingBaseUrl, "http://", ""), "https://", ""))
	}
	if common.SydneyBaseUrl != "" {
		chat.SetSydneyBaseUrl(strings.ReplaceAll(strings.ReplaceAll(common.SydneyBaseUrl, "http://", ""), "https://", ""))
	}

	err = chat.NewConversation()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	chat.SetStyle(resq.Model)

	prompt, msg := chat.MsgComposer(resq.Messages)
	resp := chatResponse{
		Id:                "chatcmpl-NewBing",
		Object:            "chat.completion.chunk",
		SystemFingerprint: hex.NewHex(12),
		Model:             resq.Model,
		Create:            time.Now().Unix(),
	}

	if resq.Stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")

		text := make(chan string)
		go chat.ChatStream(prompt, msg, text)
		var tmp string

		for {
			tmp = <-text
			resp.Choices = []choices{
				{
					Index: 0,
					Delta: binglib.Message{
						// Role:    "assistant",
						Content: tmp,
					},
				},
			}
			if tmp == "EOF" {
				resp.Choices[0].Delta.Content = ""
				resp.Choices[0].FinishReason = &STOPFLAG
				resData, err := json.Marshal(resp)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					return
				}
				w.Write([]byte("data: "))
				w.Write(resData)
				break
			}
			resData, err := json.Marshal(resp)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}
			w.Write([]byte("data: "))
			w.Write(resData)
			w.Write([]byte("\n\n"))
			flusher.Flush()

			if tmp == "User needs to solve CAPTCHA to continue." {
				if common.BypassServer != "" {
					go func(cookie string) {
						t, _ := getCookie(cookie)
						if t != "" {
							globalChat.SetCookies(t)
						}
					}(globalChat.GetCookies())
				}
			}
		}
	} else {
		text, err := chat.Chat(prompt, msg)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		resp.Choices = append(resp.Choices, choices{
			Index: 0,
			Message: binglib.Message{
				Role:    "assistant",
				Content: text,
			},
			FinishReason: &STOPFLAG,
		})

		resData, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(resData)

		if text == "User needs to solve CAPTCHA to continue." {
			if common.BypassServer != "" {
				go func(cookie string) {
					t, _ := getCookie(cookie)
					if t != "" {
						globalChat.SetCookies(t)
					}
				}(globalChat.GetCookies())
			}
		}
	}
}

func ImageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if apikey != "" {
		if r.Header.Get("Authorization") != "Bearer "+apikey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	image := globalImage.Clone()

	cookie := r.Header.Get("Cookie")
	if cookie == "" {
		if len(common.USER_TOKEN_LIST) > 0 {
			seed := time.Now().UnixNano()
			rng := rand.New(rand.NewSource(seed))
			cookie = common.USER_TOKEN_LIST[rng.Intn(len(common.USER_TOKEN_LIST))]
		} else {
			if common.BypassServer != "" {
				t, _ := getCookie(cookie)
				if t != "" {
					cookie = t
				}
			}
		}
	}
	image.SetCookies(cookie)

	resqB, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	var resq imageRequest
	json.Unmarshal(resqB, &resq)

	if common.BingBaseUrl != "" {
		image.SetBingBaseUrl(strings.ReplaceAll(strings.ReplaceAll(common.BingBaseUrl, "http://", ""), "https://", ""))
	}
	imgs, _, err := image.Image(resq.Prompt)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	resp := imageResponse{
		Created: time.Now().Unix(),
	}
	for _, img := range imgs {
		resp.Data = append(resp.Data, imageData{
			Url: img,
		})
	}

	resData, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(resData)
}

func ModelsHandler(w http.ResponseWriter, r *http.Request) {

}

func getCookie(reqCookie string) (cookie string, err error) {
	cookie = reqCookie
	c := request.NewRequest()
	res := c.SetUrl(common.BingBaseUrl+"/search?q=Bing+AI&showconv=1&FORM=hpcodx&ajaxhist=0&ajaxserp=0&cc=us").
		SetHeader("User-Agent", common.User_Agent).
		SetHeader("Cookie", cookie).Do()
	headers := res.GetHeaders()
	for k, v := range headers {
		if strings.ToLower(k) == "set-cookie" {
			for _, i := range v {
				cookie += strings.Split(i, "; ")[0] + "; "
			}
		}
	}
	cookie = strings.TrimLeft(strings.Trim(cookie, "; "), "; ")
	resp, err := api.Bypass(common.BypassServer, cookie, "local-gen-"+hex.NewUUID())
	if err != nil {
		return
	}
	return resp.Result.Cookies, nil
}
