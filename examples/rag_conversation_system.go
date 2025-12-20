/**
 * RAG Conversation System Example
 *
 * This example demonstrates building a complete RAG (Retrieval-Augmented Generation) system
 * where AI can search across all previous conversations to provide context-aware responses.
 *
 * Key Features:
 * - Store messages with automatic embeddings
 * - Search related messages across all conversations
 * - Use search results as context for AI responses
 * - Dynamic conversation configurations
 * - Cross-conversation knowledge retrieval
 *
 * This showcases how ekoDB can be the complete backend for a self-improving AI chat system
 * that learns from its own history.
 *
 * Prerequisites:
 * - Run the ekoDB server: make run
 * - Set OPENAI_API_KEY environment variable
 *
 * Run with: go run examples/rag_conversation_system.go
 */

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	ekodb "github.com/ekoDB/ekodb-client-go"
	"github.com/joho/godotenv"
)

type Message struct {
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	Embedding      []float64 `json:"embedding"`
	Tags           string    `json:"tags"`
	Timestamp      string    `json:"timestamp"`
}

func extractStringField(record ekodb.Record, field string) string {
	if val, ok := record[field]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
		if mapVal, ok := val.(map[string]interface{}); ok {
			if value, ok := mapVal["value"].(string); ok {
				return value
			}
		}
	}
	return "N/A"
}

func createConversation(client *ekodb.Client, collection, convID, title string) error {
	conv := ekodb.Record{
		"id":         convID,
		"title":      title,
		"created_at": time.Now().Format(time.RFC3339),
		"search_config": map[string]interface{}{
			"collections": []string{"rag_messages"},
			"search_type": "hybrid",
			"limit":       10,
		},
	}
	_, err := client.Insert(collection, conv)
	return err
}

func storeMessageWithEmbedding(client *ekodb.Client, collection, conversationID, role, content string, tags []string) error {
	fmt.Println("  → Calling ekoDB Embed() helper...")
	fmt.Println("    • Using model: text-embedding-3-small")
	fmt.Printf("    • Text length: %d characters\n", len(content))
	fmt.Println("    • Behind the scenes: Creating temp Function with Embed operation")

	start := time.Now()
	embedding, err := client.Embed(content, "text-embedding-3-small")
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}
	duration := time.Since(start).Seconds()

	fmt.Printf("    ✓ Generated embedding: %d dimensions in %.3fs\n", len(embedding), duration)
	fmt.Println("    • Function auto-cleaned up by client")

	tagsStr := ""
	for i, tag := range tags {
		if i > 0 {
			tagsStr += ","
		}
		tagsStr += tag
	}

	msg := ekodb.Record{
		"conversation_id": conversationID,
		"role":            role,
		"content":         content,
		"embedding":       embedding,
		"tags":            tagsStr,
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	_, err = client.Insert(collection, msg)
	return err
}

func main() {
	fmt.Println("=== ekoDB RAG Conversation System ===\n")
	fmt.Println("This example shows how ekoDB can power a self-improving AI system")
	fmt.Println("that learns from its own conversation history.\n")

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using defaults")
	}

	// Create client
	baseURL := os.Getenv("API_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	apiKey := os.Getenv("API_BASE_KEY")
	if apiKey == "" {
		apiKey = "a-test-api-key-from-ekodb"
	}

	client, err := ekodb.NewClient(baseURL, apiKey)
	if err != nil {
		log.Fatal(err)
	}

	messagesCollection := "rag_messages"
	conversationsCollection := "rag_conversations"

	// Cleanup any existing data
	client.DeleteCollection(messagesCollection)
	client.DeleteCollection(conversationsCollection)

	// ========================================
	// STEP 1: Simulate Historical Conversations
	// ========================================
	fmt.Println("=== Step 1: Building Conversation History ===")
	fmt.Println("Storing previous conversations with embeddings...\n")

	// Conversation 1: Rust Programming Discussion
	conv1ID := "conv_rust_programming"
	if err := createConversation(client, conversationsCollection, conv1ID, "Rust Programming"); err != nil {
		log.Fatal(err)
	}

	rustMessages := []struct {
		role    string
		content string
	}{
		{"user", "What are the key features of Rust?"},
		{"assistant", "Rust's key features include: memory safety without garbage collection, zero-cost abstractions, ownership system, powerful type system, and excellent concurrency support."},
		{"user", "How does the borrow checker work?"},
		{"assistant", "The borrow checker enforces Rust's ownership rules at compile time. It ensures that references don't outlive the data they point to and prevents data races by allowing either multiple immutable references or one mutable reference."},
	}

	for _, msg := range rustMessages {
		if err := storeMessageWithEmbedding(client, messagesCollection, conv1ID, msg.role, msg.content, []string{"rust", "programming"}); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("✓ Stored Rust programming conversation (%d messages)\n", len(rustMessages))

	// Conversation 2: Database Design Discussion
	conv2ID := "conv_database_design"
	if err := createConversation(client, conversationsCollection, conv2ID, "Database Design"); err != nil {
		log.Fatal(err)
	}

	dbMessages := []struct {
		role    string
		content string
	}{
		{"user", "What is database normalization?"},
		{"assistant", "Database normalization is the process of organizing data to reduce redundancy and improve data integrity. It involves dividing large tables into smaller ones and defining relationships between them using foreign keys."},
		{"user", "When should I use NoSQL over SQL?"},
		{"assistant", "Use NoSQL when you need: flexible schemas, horizontal scaling, high write throughput, or when working with unstructured data. SQL is better for complex queries, ACID transactions, and structured data with well-defined relationships."},
	}

	for _, msg := range dbMessages {
		if err := storeMessageWithEmbedding(client, messagesCollection, conv2ID, msg.role, msg.content, []string{"database", "design"}); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("✓ Stored database design conversation (%d messages)\n", len(dbMessages))

	// Conversation 3: Performance Optimization
	conv3ID := "conv_performance"
	if err := createConversation(client, conversationsCollection, conv3ID, "Performance Optimization"); err != nil {
		log.Fatal(err)
	}

	perfMessages := []struct {
		role    string
		content string
	}{
		{"user", "How can I optimize database queries?"},
		{"assistant", "Key database optimization techniques: use indexes wisely, avoid SELECT *, optimize JOIN operations, use query caching, denormalize when needed, and analyze query execution plans."},
		{"user", "What about memory management in Rust?"},
		{"assistant", "Rust's ownership system provides zero-cost memory management. Use Box for heap allocation, Rc/Arc for shared ownership, and avoid cloning large data structures. The compiler optimizes away unnecessary allocations."},
	}

	for _, msg := range perfMessages {
		if err := storeMessageWithEmbedding(client, messagesCollection, conv3ID, msg.role, msg.content, []string{"performance", "optimization"}); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("✓ Stored performance optimization conversation (%d messages)\n\n", len(perfMessages))

	// ========================================
	// STEP 2: New User Question - RAG in Action
	// ========================================
	fmt.Println("=== Step 2: New User Question with Context Retrieval ===")
	userQuestion := "How do I write memory-safe high-performance database code?"
	fmt.Printf("User asks: \"%s\"\n\n", userQuestion)

	// ========================================
	// STEP 3: Search Across ALL Previous Conversations
	// ========================================
	fmt.Println("=== Step 3: Searching Related Context ===")
	fmt.Println("Using hybrid search to find relevant messages from all conversations...\n")

	// Generate embedding for the question
	fmt.Println("\n→ Generating embedding for user question...")
	questionEmbedding, err := func() ([]float64, error) {
		fmt.Println("  → Calling ekoDB Embed() helper...")
		fmt.Println("    • Using model: text-embedding-3-small")
		fmt.Printf("    • Text length: %d characters\n", len(userQuestion))
		fmt.Println("    • Behind the scenes: Creating temp Function with Embed operation")

		start := time.Now()
		emb, err := client.Embed(userQuestion, "text-embedding-3-small")
		if err != nil {
			return nil, err
		}
		duration := time.Since(start).Seconds()

		fmt.Printf("    ✓ Generated embedding: %d dimensions in %.3fs\n", len(emb), duration)
		fmt.Println("    • Function auto-cleaned up by client")

		return emb, nil
	}()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n→ Executing HybridSearch()...")
	fmt.Printf("  • Collection: %s\n", messagesCollection)
	fmt.Printf("  • Query text: \"%s\"\n", userQuestion)
	fmt.Printf("  • Vector dimensions: %d\n", len(questionEmbedding))
	fmt.Println("  • Limit: 5 results")
	fmt.Println("  • Search type: Semantic (vector) + Keyword (text)")
	fmt.Println("  • Server combines both scores for relevance ranking")

	searchStart := time.Now()
	relatedMessages, err := client.HybridSearch(messagesCollection, userQuestion, questionEmbedding, 5)
	if err != nil {
		log.Fatal(err)
	}
	searchDuration := time.Since(searchStart).Seconds()
	fmt.Printf("  ✓ Search completed in %.3fs\n", searchDuration)

	fmt.Printf("\n✓ Found %d related messages across all conversations:\n", len(relatedMessages))

	contextMessages := []string{}
	for i, msg := range relatedMessages {
		content := extractStringField(msg, "content")
		convID := extractStringField(msg, "conversation_id")
		score := 0.0
		if s, ok := msg["_score"].(float64); ok {
			score = s
		}

		fmt.Printf("  %d. [Score: %.3f] From %s\n", i+1, score, convID)
		fmt.Printf("     %s\n\n", content)

		contextMessages = append(contextMessages, content)
	}

	// ========================================
	// STEP 4: Build Context-Aware AI Response
	// ========================================
	fmt.Println("=== Step 4: Generating Context-Aware Response ===")

	// Prepare context from search results
	context := "Here is relevant information from previous conversations:\n\n"
	for i, msg := range contextMessages {
		context += fmt.Sprintf("Context %d: %s\n\n", i+1, msg)
	}

	// Create chat session for the new question
	systemPrompt := fmt.Sprintf("You are a helpful programming assistant. Use the provided context to give comprehensive answers that combine knowledge from multiple topics. Context:\n\n%s", context)

	chatSession, err := client.CreateChatSession(ekodb.CreateChatSessionRequest{
		Collections:  []ekodb.CollectionConfig{},
		LLMProvider:  "openai",
		LLMModel:     strPtr("gpt-4"),
		SystemPrompt: &systemPrompt,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Send the question
	response, err := client.ChatMessage(chatSession.ChatID, ekodb.ChatMessageRequest{
		Message: userQuestion,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ AI Response (with context from 3 conversations):\n")
	if len(response.Responses) > 0 {
		fmt.Printf("%s\n\n", response.Responses[0])
	}

	// ========================================
	// STEP 5: Store New Conversation
	// ========================================
	fmt.Println("=== Step 5: Storing New Conversation ===")

	newConvID := "conv_new_question"
	if err := createConversation(client, conversationsCollection, newConvID, "Memory-Safe Database Code"); err != nil {
		log.Fatal(err)
	}

	// Store user question
	if err := storeMessageWithEmbedding(client, messagesCollection, newConvID, "user", userQuestion, []string{"rust", "database", "performance"}); err != nil {
		log.Fatal(err)
	}

	// Store AI response
	if len(response.Responses) > 0 {
		if err := storeMessageWithEmbedding(client, messagesCollection, newConvID, "assistant", response.Responses[0], []string{"rust", "database", "performance"}); err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("✓ New conversation stored and indexed for future retrieval\n")

	// ========================================
	// STEP 6: Demonstrate Cross-Conversation Search
	// ========================================
	fmt.Println("=== Step 6: Cross-Conversation Search ===")
	fmt.Println("Searching for messages about 'ownership' across ALL conversations...\n")

	fmt.Println("\n→ Executing TextSearch()...")
	fmt.Printf("  • Collection: %s\n", messagesCollection)
	fmt.Println("  • Query: \"ownership system\"")
	fmt.Println("  • Limit: 3 results")
	fmt.Println("  • Search method: Full-text with fuzzy matching & stemming")
	fmt.Println("  • No vector embeddings needed - pure keyword search")

	textStart := time.Now()
	ownershipResults, err := client.TextSearch(messagesCollection, "ownership system", 3)
	if err != nil {
		log.Fatal(err)
	}
	textDuration := time.Since(textStart).Seconds()
	fmt.Printf("  ✓ Text search completed in %.3fs\n", textDuration)

	fmt.Printf("\n✓ Found %d messages mentioning ownership:\n", len(ownershipResults))
	for i, msg := range ownershipResults {
		content := extractStringField(msg, "content")
		convID := extractStringField(msg, "conversation_id")
		fmt.Printf("  %d. From %s: %s\n\n", i+1, convID, content)
	}

	// ========================================
	// STEP 7: Show System Statistics
	// ========================================
	fmt.Println("=== System Statistics ===\n")

	fmt.Println("→ Querying database statistics...")
	fmt.Println("  • Using FindAll() helper - simplified query API\n")

	totalMessages, err := client.FindAll(messagesCollection, 1000)
	if err != nil {
		log.Fatal(err)
	}
	totalConvs, err := client.FindAll(conversationsCollection, 100)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("📊 Database Statistics:")
	fmt.Printf("  • Total conversations: %d\n", len(totalConvs))
	fmt.Printf("  • Total messages stored: %d\n", len(totalMessages))
	fmt.Println("  • All messages indexed for vector search ✓")
	fmt.Println("  • All messages indexed for text search ✓")
	fmt.Println("  • All messages queryable by metadata ✓\n")

	// ========================================
	// STEP 8: Demonstrate Dynamic Configuration
	// ========================================
	fmt.Println("=== Step 8: Dynamic Search Configuration ===")
	fmt.Println("Each conversation can have its own search config...\n")

	fmt.Println("💡 Conversations can store custom search configurations:")
	fmt.Println("  • Search type: hybrid, text, or vector")
	fmt.Println("  • Relevance thresholds")
	fmt.Println("  • Filter by tags or metadata")
	fmt.Println("  • Collection-specific settings")
	fmt.Println("  • Per-conversation AI behavior")
	fmt.Println("\nThis enables context-aware search tuned to each conversation's needs!\n")

	// ========================================
	// Cleanup
	// ========================================
	fmt.Println("=== Cleanup ===")
	if err := client.DeleteChatSession(chatSession.ChatID); err != nil {
		log.Printf("Warning: failed to delete chat session: %v", err)
	}
	client.DeleteCollection(messagesCollection)
	client.DeleteCollection(conversationsCollection)
	fmt.Println("✓ Cleanup complete\n")

	// ========================================
	// Summary
	// ========================================
	fmt.Println("\n=== 📚 Summary: What This Example Showed ===\n")
	fmt.Println("🔧 ekoDB Native Capabilities Used:")
	fmt.Println("  ✓ Functions with Embed operation (AI integration)")
	fmt.Println("  ✓ Hybrid Search (text + vector combined)")
	fmt.Println("  ✓ Text Search (full-text with stemming)")
	fmt.Println("  ✓ Automatic embedding generation")
	fmt.Println("  ✓ Cross-collection queries\n")
	fmt.Println("🚀 New Client Helper Methods:")
	fmt.Println("  • client.Embed(text, model) - Generate embeddings")
	fmt.Println("  • client.HybridSearch() - Semantic + keyword search")
	fmt.Println("  • client.TextSearch() - Full-text search")
	fmt.Println("  • client.FindAll() - Query all documents\n")
	fmt.Println("💡 Key Takeaways:")
	fmt.Println("  1. ekoDB handles AI Functions natively - no external services needed")
	fmt.Println("  2. One-line embedding generation with auto-cleanup")
	fmt.Println("  3. Hybrid search combines semantic understanding + keyword matching")
	fmt.Println("  4. Perfect for RAG: store, search, and retrieve context")
	fmt.Println("  5. All AI capabilities accessible through simple client methods\n")
	fmt.Println("🎯 Build production RAG systems with ekoDB!")
	fmt.Println("   → Set OPENAI_API_KEY in your ekoDB server environment")
	fmt.Println("   → Use these client helpers to make AI integration simple")
	fmt.Println("   → Scale to millions of documents with native indexing\n")
}

func strPtr(s string) *string {
	return &s
}
