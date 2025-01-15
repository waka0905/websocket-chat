package main

import (
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

	client := &Client{conn: conn, send: make(chan []byte)}
	mutex.Lock()
	clients[client] = true
	mutex.Unlock()

	// 非同期でメッセージを送信
	go client.writeMessage()

	fmt.Println("New WebSocket connection")

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
		fmt.Println("Received message: ", string(msg))

		// メッセージをブロードキャスト
		broadcast <- msg
	}
}

// メッセージを全クライアントに送信
func handleMessages() {
	for {
		msg := <-broadcast
		mutex.Lock()
		for client := range clients {
			select {
			case client.send <- msg:
				// メッセージ送信成功
			default:
				// メッセージ送信に失敗した場合、クライアントを削除
				delete(clients, client)
				close(client.send) // チャネルを閉じる
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
