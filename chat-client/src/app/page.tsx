"use client";
import axios from "axios";
import { v4 as uuidv4 } from "uuid";
import { useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";

const API_URL = "http://localhost:8001";

type ChatResponse = {
  role: "user" | "ai";
  content: string;
};

export default function Home() {
  const [userId, setUserId] = useState<string>("");
  const [message, setMessage] = useState<string>("");
  const [chatHistory, setChatHistory] = useState<Array<ChatResponse>>([]);
  const [isTyping, setIsTyping] = useState<boolean>(false);
  const [eventSource, setEventSource] = useState<EventSource | null>(null);
  const chatEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let storedUserId = sessionStorage.getItem("userId");
    if (!storedUserId) {
      storedUserId = uuidv4();
      sessionStorage.setItem("userId", storedUserId);
    }
    setUserId(storedUserId);
    fetchContext(storedUserId);
  }, []);

  const updateContext = async () => {
    try {
      await axios.post(`${API_URL}/context`, {
        user_id: userId,
        messages: [],
      });
      fetchContext(userId);
    } catch (e: unknown) {
      console.error("Error updating context: ", e);
    }
  };

  const fetchContext = async (userId: string) => {
    try {
      const res = await axios.get(`${API_URL}/context/${userId}`);
      setChatHistory(res.data.context.messages || []);
    } catch (e: unknown) {
      console.log("Error fetching context", e);
    }
  };

  const sendMessage = async () => {
    if (!message.trim()) return;

    setChatHistory((prev) => [...prev, { role: "user", content: message }]);
    setMessage("");
    setIsTyping(true);

    const newEventSource = new EventSource(
      `${API_URL}/chat?user_id=${userId}&message=${encodeURIComponent(message)}`,
    );
    setEventSource(newEventSource);

    let chatResponse = "";

    newEventSource.onmessage = (event) => {
      chatResponse += event.data;
      setChatHistory((prev) => {
        const lastMessage = prev[prev.length - 1];
        if (lastMessage?.role === "ai") {
          return [
            ...prev.slice(0, -1),
            { role: "ai", content: chatResponse.trim() },
          ];
        }

        return [...prev, { role: "ai", content: chatResponse.trim() }];
      });
    };

    newEventSource.onerror = () => {
      newEventSource.close();
      setIsTyping(false);
      setEventSource(null);
    };
  };

  const stopMessage = () => {
    if (eventSource) {
      eventSource.close();
      setIsTyping(false);
      setEventSource(null);
    }
  };

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [chatHistory, isTyping]);

  return (
    <div className="flex flex-col items-center p-8 bg-grey-100 min-h-screen">
      <h1 className="text-2xl font-bold mb-4">Mistral Chat</h1>
      <div className="w-full bg-white shadow-md rounded-lg p-4 overflow-y-auto h-100">
        {chatHistory.map((msg, index) => (
          <div
            key={index}
            className={`p-2 my-1 rounded-lg w-fit max-w-xs ${msg.role === "user" ? "bg-blue-500 text-white self-end" : "bg-gray-300 text-black self-start"}`}
          >
            <ReactMarkdown>{msg.content}</ReactMarkdown>
          </div>
        ))}
        {isTyping && <div className="text-gray-500 italic">Typing...</div>}
        <div ref={chatEndRef}></div>
      </div>
      <div className="mt-4 flex w-full max-w-lg">
        <textarea
          className="flex-1 border p-2 rounded-l-lg resize-node h-20"
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          placeholder="Type a message..."
        />
        <button
          className="bg-blue-500 text-white p-2 rounded-r-lg"
          onClick={sendMessage}
        >
          Send
        </button>
        {isTyping && (
          <button
            className="bg-red-500 text-white p-2 rounded ml-2"
            onClick={stopMessage}
          >
            Stop
          </button>
        )}
      </div>
      <button
        className="mt-2 text-sm text-gray-600 underline"
        onClick={updateContext}
      >
        Reset chat
      </button>
    </div>
  );
}
