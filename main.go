package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocketのアップグレーダー
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 全ての接続を許可
	},
}

// クライアントを管理するための構造体
type Client struct {
	conn *websocket.Conn
	send chan []byte
	id   string // クライアントを識別するためのID（アドレス等でも可）
}

type Message struct {
	Sender  string `json:"sender"`
	Content string `json:"content"`
	Time    string `json:"time"`
}

// 接続中のクライアントを管理
var clients = make(map[*Client]bool)
var broadcast = make(chan []byte)
var mutex = &sync.Mutex{}

func main() {
	// HTTPハンドラーを設定
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", handleConnections)

	// メッセージをブロードキャストする並行処理
	go handleMessages()

	fmt.Println("WebSocket chat server started on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}

// ホームページを提供
func serveHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

// WebSocket接続を処理
func handleConnections(w http.ResponseWriter, r *http.Request) {
	// WebSocketにアップグレード
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading to WebSocket:", err)
		return
	}
	defer conn.Close()

	// クライアントを識別するためのIDとして接続元のアドレスを利用
	client := &Client{
		conn: conn,
		send: make(chan []byte),
		id:   conn.RemoteAddr().String(),
	}

	// クライアントをリストに追加
	mutex.Lock()
	clients[client] = true
	mutex.Unlock()

	// 非同期でメッセージを送信
	go client.writeMessage()

	fmt.Println("New WebSocket connection from", client.id)

	for {
		// メッセージを読み取る
		_, msg, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Error reading message:", err)
			mutex.Lock()
			delete(clients, client)
			mutex.Unlock()
			break
		}

		// メッセージをサーバー側でログ出力
		fmt.Println("Received message from", client.id, ": ", string(msg))

		// メッセージをサーバーに渡す
		message := Message{
			Sender:  client.id,
			Content: string(msg),
			// Time:    time.Now().Format(time.RFC3339),
		}
		// メッセージをJSON形式にエンコード
		encodedMsg, err := json.Marshal(message)
		if err != nil {
			fmt.Println("Error marshalling message:", err)
			continue
		}

		// メッセージをブロードキャスト
		broadcast <- encodedMsg
	}
}

// メッセージを全クライアントに送信（送信元のクライアントには送信しない）
func handleMessages() {
	for {
		msg := <-broadcast
		mutex.Lock()
		for client := range clients {
			// メッセージ送信元のクライアントを除外
			// msg.Sender と client.id を比較して送信元を判定
			var message Message
			err := json.Unmarshal(msg, &message)
			if err != nil {
				fmt.Println("Error unmarshalling message:", err)
				continue
			}

			if message.Sender != client.id { // 自分には送信しない
				select {
				case client.send <- msg:
				default:
					delete(clients, client)
					close(client.send) // チャネルを閉じる
				}
			}
		}
		mutex.Unlock()
	}
}

// メッセージ送信
func (c *Client) writeMessage() {
	for msg := range c.send {
		err := c.conn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			fmt.Println("Error sending message:", err)
			c.conn.Close()
			break
		}
	}
}
