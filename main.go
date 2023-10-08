package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

var todoLists = make(map[int]*TodoList)
var listIDCounter = 0
var mutex sync.RWMutex

type TodoList struct {
	Id    int    `json:"id"`
	Title string `json:"title"`
	List  []struct {
		ItemID    int    `json:"itemid"`
		Detail    string `json:"detail"`
		Completed bool   `json:"completed"`
	} `json:"list"`
}

func formatItemList(items *TodoList) string {
	var formattedItems []string
	for _, item := range items.List {
		formattedItems = append(formattedItems, fmt.Sprintf(`{
			"itemid": %v,
			"detail": "%v",
			"completed": %t
		}`, item.ItemID, item.Detail, item.Completed))
	}
	return "[" + strings.Join(formattedItems, ",") + "]"
}

func handleGetRequest(listID int) string {
	mutex.RLock()
	defer mutex.RUnlock()
	list, exists := todoLists[listID]
	if !exists {
		return `{}`
	}

	response := fmt.Sprintf(`{
		"id": %d,
		"title": "%s",
		"list": %s
	}`, listID, list.Title, formatItemList(list))

	return response
}

func handleDeleteRequest(listID int) {
	mutex.Lock()
	defer mutex.Unlock()

	delete(todoLists, listID)
}

func handlePostNewListRequest(request *TodoList) string {
	mutex.Lock()
	defer mutex.Unlock()

	listIDCounter++
	todoLists[listIDCounter] = request

	return fmt.Sprintf(`{"id":%d}`, listIDCounter)
}

func handlePostListRequest(request *TodoList) {
	mutex.Lock()
	defer mutex.Unlock()

	_, exists := todoLists[request.Id]
	if !exists {
		return
	}

	todoLists[request.Id] = request
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Read request
	request := make([]byte, 1024 * 1024)
	_, err := conn.Read(request)
	if err == io.EOF {
		return
	}
	if err != nil {
		fmt.Println("Error reading request:", err)
		sendResponse(conn, 500, "Internal Server Error", "")
		return
	}

	// Parse request method and URL
	requestString := string(request)
	requestLines := strings.SplitN(requestString, "\r\n", 2)
	requestLine := strings.Split(requestLines[0], " ")
	method := requestLine[0]
	url := requestLine[1]

	switch {
	case method == "GET" && strings.HasPrefix(url, "/api/list"):
		listID := 0
		n, err := fmt.Sscanf(url, "/api/list/%d", &listID)
		if n != 1 || err != nil {
			fmt.Println("Error parsing request:", err)
			sendResponse(conn, 400, "Bad Request", "")
			return
		}
		sendResponse(conn, 200, "OK", handleGetRequest(listID))

	case method == "POST" && url == "/api/list/new":
		requestBody := strings.SplitN(requestLines[1], "\r\n\r\n", 2)[1]
		requestBody = strings.Trim(requestBody, string('\x00'))
		fmt.Println("Received Request Body:", requestBody)
		var newListRequest TodoList
		if err := json.Unmarshal([]byte(requestBody), &newListRequest); err != nil {
			fmt.Println("Error parsing request body:", err)
			sendResponse(conn, 400, "Bad Request", "")
			return
		}

		sendResponse(conn, 200, "OK", handlePostNewListRequest(&newListRequest))

	case method == "POST" && strings.HasPrefix(url, "/api/list"):
		listID := 0
		n, err := fmt.Sscanf(url, "/api/list/%d", &listID)
		if n != 1 || err != nil {
			fmt.Println("Error parsing request:", err)
			sendResponse(conn, 400, "Bad Request", "")
			return
		}

		requestBody := strings.SplitN(requestLines[1], "\r\n\r\n", 2)[1]
		requestBody = strings.Trim(requestBody, string('\x00'))
		fmt.Println("Received Request Body:", requestBody)
		var modifyListRequest TodoList
		if err := json.Unmarshal([]byte(requestBody), &modifyListRequest); err != nil || modifyListRequest.Id != listID {
			fmt.Println("Error parsing request body:", err)
			sendResponse(conn, 400, "Bad Request", "")
			return
		}

		handlePostListRequest(&modifyListRequest)
		sendResponse(conn, 200, "OK", "")

	case method == "DELETE" && strings.HasPrefix(url, "/api/list"):
		listID := 0
		n, err := fmt.Sscanf(url, "/api/list/%d", &listID)
		if n != 1 || err != nil {
			fmt.Println("Error parsing request:", err)
			sendResponse(conn, 400, "Bad Request", "")
			return
		}

		handleDeleteRequest(listID)
		sendResponse(conn, 200, "OK", "")
	default:
		sendResponse(conn, 404, "Not Found", "")
	}
}

func sendResponse(conn net.Conn, code int, msg, body string) {
	res := fmt.Sprintf("HTTP/1.1 %d %s\r\n Content-Length: %d\r\n\r\n%s", code, msg, len(body), body)
	_, err := conn.Write([]byte(res))
	if err != nil {
		fmt.Println("Error writing response:", err)
		return
	}
}

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer listener.Close()

	fmt.Println("Server started. Listening on :8080")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		go handleConnection(conn)
	}
}
