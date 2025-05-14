package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

// Document represents a judgment document
type Document struct {
	ID      int    `json:"id"` // 注意：在部分数据库表中，该字段可能不存在
	Title   string `json:"title"`
	Content string `json:"content"`
	Source  string `json:"source"` // 来源: "台州中院", "温岭法院", "已导入数据"
	Type    string `json:"type"`   // 类型: "刑事", "民事", "其他"
}

// Database connections and document cache
var (
	tzDB   *sql.DB
	wlDB   *sql.DB
	userDB *sql.DB
	
	// 文档缓存，对于没有ID列的数据库表使用
	documentCache map[string]Document = make(map[string]Document)
	cacheMutex sync.Mutex
)

func main() {
	// Initialize databases
	var err error

	// Open 台州市 database
	tzDB, err = sql.Open("sqlite3", "tz-2020.db")
	if err != nil {
		log.Fatalf("Failed to open tz-2020.db: %v", err)
	}
	defer tzDB.Close()

	// Open 温岭市 database
	wlDB, err = sql.Open("sqlite3", "wl-2020.db")
	if err != nil {
		log.Fatalf("Failed to open wl-2020.db: %v", err)
	}
	defer wlDB.Close()

	// Create or open user-imported database
	err = os.MkdirAll("data", 0755)
	if err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	userDB, err = sql.Open("sqlite3", "data/user_imported.db")
	if err != nil {
		log.Fatalf("Failed to open user_imported.db: %v", err)
	}
	defer userDB.Close()

	// 创建表(如果不存在) - 不使用id列且不重置表
	_, err = userDB.Exec(`
		CREATE TABLE IF NOT EXISTS documents (
			title TEXT,
			content TEXT
		)
	`)
	if err != nil {
		log.Fatalf("Failed to create table in user_imported.db: %v", err)
	}

	// Initialize Gin router
	router := gin.Default()

	// Serve static files
	router.Static("/static", "./static")
	router.LoadHTMLGlob("templates/*")

	// Routes
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"title": "判决书检索工具",
		})
	})

	// API routes
	api := router.Group("/api")
	{
		api.GET("/search", searchDocuments)
		api.GET("/document/:id", getDocument)
		api.POST("/upload", uploadDocument)
		api.POST("/clear-db", clearUserDatabase)
	}

	// Start server
	// 监听在所有网络接口(0.0.0.0)的端口8081上
	// 执行 ifconfig | grep inet 或在网络设置中查看你的IP地址
	ip := "0.0.0.0"
	port := "8081"
	fmt.Printf("Server running on all interfaces at http://%s:%s\n", ip, port)
	fmt.Println("Your colleagues can access this application using your computer's IP address.")
	fmt.Println("For example: http://YOUR_IP_ADDRESS:8081")
	router.Run(ip + ":" + port)
}

// searchDocuments handles the search functionality
func searchDocuments(c *gin.Context) {
	// 清空上一次搜索的文档缓存
	cacheMutex.Lock()
	documentCache = make(map[string]Document)
	cacheMutex.Unlock()
	
	query := c.Query("q")
	excludeQuery := c.Query("exclude")
	searchType := c.Query("type")      // "title", "content", "all"
	docType := c.Query("docType")      // "刑事", "民事", "全部"
	sourceDB := c.Query("source")      // "台州中院", "温岭法院", "已导入数据", "全部"
	
	log.Printf("Search request - query: '%s', exclude: '%s', type: '%s', docType: '%s', source: '%s'", 
		query, excludeQuery, searchType, docType, sourceDB)
	
	if searchType == "" {
		searchType = "content" // Default to content search
	}
	
	if docType == "" {
		docType = "全部" // Default to all document types
	}
	
	if sourceDB == "" {
		sourceDB = "全部" // Default to all sources
	}

	// Split query into keywords
	keywords := strings.Fields(query)
	excludeKeywords := strings.Fields(excludeQuery)

	log.Printf("Parsed keywords: %v, exclude: %v", keywords, excludeKeywords)
	
	var results []Document
	var err error

	// Search in the selected databases
	if sourceDB == "全部" || sourceDB == "台州中院 2020 前" {
		var tzResults []Document
		tzResults, err = searchInDB(tzDB, keywords, excludeKeywords, searchType, docType, "台州中院")
		if err != nil {
			log.Printf("Error searching in 台州中院 database: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库搜索出错: " + err.Error()})
			return
		}
		results = append(results, tzResults...)
	}

	if sourceDB == "全部" || sourceDB == "温岭法院 2020 前" {
		var wlResults []Document
		wlResults, err = searchInDB(wlDB, keywords, excludeKeywords, searchType, docType, "温岭法院")
		if err != nil {
			log.Printf("Error searching in 温岭法院 database: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库搜索出错: " + err.Error()})
			return
		}
		results = append(results, wlResults...)
	}

	if sourceDB == "全部" || sourceDB == "已导入数据" {
		var userResults []Document
		userResults, err = searchInDB(userDB, keywords, excludeKeywords, searchType, docType, "已导入数据")
		if err != nil {
			log.Printf("Error searching in 已导入数据 database: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库搜索出错: " + err.Error()})
			return
		}
		results = append(results, userResults...)
	}

	log.Printf("Search completed, found %d results", len(results))
	c.JSON(http.StatusOK, results)
}

// searchInDB performs the search in a specific database
func searchInDB(db *sql.DB, keywords, excludeKeywords []string, searchType, docType, source string) ([]Document, error) {
	var results []Document

	// First, let's check the table structure
	columns, err := getTableColumns(db, "documents")
	if err != nil {
		log.Printf("Error getting table columns for database %s: %v", source, err)
		return nil, fmt.Errorf("获取%s数据库表结构出错: %w", source, err)
	}

	// Determine if we have an id column
	hasIDColumn := contains(columns, "id")
	log.Printf("Database %s has ID column: %v", source, hasIDColumn)

	// Build the query based on table structure and search parameters
	var query string
	if hasIDColumn {
		query = "SELECT id, title, content FROM documents WHERE 1=1"
	} else {
		query = "SELECT title, content FROM documents WHERE 1=1"
	}
	
	// Add search conditions for keywords
	var args []interface{}

	if len(keywords) > 0 {
		// 为每个关键词构建条件组
		var keywordGroupConditions []string
		
		for _, keyword := range keywords {
			// Log the keyword being processed
			log.Printf("Processing keyword: '%s' for database: %s", keyword, source)
			
			// 当前关键词的条件（标题或内容包含该关键词）
			var keywordConditions []string
			
			// 根据搜索类型添加条件
			if searchType == "title" || searchType == "all" {
				keywordConditions = append(keywordConditions, "title LIKE ?")
				args = append(args, "%"+keyword+"%")
			}
			
			if searchType == "content" || searchType == "all" {
				keywordConditions = append(keywordConditions, "content LIKE ?")
				args = append(args, "%"+keyword+"%")
			}
			
			// 将当前关键词的标题和内容条件用 OR 连接
			// 例如：(title LIKE ? OR content LIKE ?)
			if len(keywordConditions) > 0 {
				keywordGroupConditions = append(keywordGroupConditions, "(" + strings.Join(keywordConditions, " OR ") + ")")
			}
		}
		
		// 将所有关键词条件组用 AND 连接，表示必须同时包含所有关键词
		if len(keywordGroupConditions) > 0 {
			query += " AND (" + strings.Join(keywordGroupConditions, " AND ") + ")"
		}
	}

	// Add exclude conditions
	if len(excludeKeywords) > 0 {
		for _, keyword := range excludeKeywords {
			log.Printf("Processing exclude keyword: '%s' for database: %s", keyword, source)
			
			if searchType == "title" || searchType == "all" {
				query += " AND title NOT LIKE ?"
				args = append(args, "%"+keyword+"%")
			}
			
			if searchType == "content" || searchType == "all" {
				query += " AND content NOT LIKE ?"
				args = append(args, "%"+keyword+"%")
			}
		}
	}

	// Add document type filter
	if docType != "全部" {
		// 判断具体是民事还是刑事案件
		switch docType {
		case "刑事":
			query += " AND title LIKE ?"
			args = append(args, "%刑%")
		case "民事":
			query += " AND title LIKE ?"
			args = append(args, "%民%")
		default:
			query += " AND title LIKE ?"
			args = append(args, "%"+docType+"%")
		}
	}

	// Log the final query and arguments
	log.Printf("Executing query in %s: %s with args: %v", source, query, args)

	// Execute the query
	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Error querying %s database: %v", source, err)
		return nil, fmt.Errorf("查询%s数据库出错: %w", source, err)
	}
	defer rows.Close()

	// Process results based on table structure
	for rows.Next() {
		var doc Document
		var scanErr error
		
		// Generate a row ID automatically if the table doesn't have an ID column
		if hasIDColumn {
			scanErr = rows.Scan(&doc.ID, &doc.Title, &doc.Content)
		} else {
			// For tables without ID column, we'll use a pseudo-ID and store in cache
			cacheID := len(results) + 1 // 使用搜索结果编号作为伪造ID
			doc.ID = cacheID
			scanErr = rows.Scan(&doc.Title, &doc.Content)
			
			// 将文档存储在缓存中，使用cacheKey便于后续精确获取
			cacheMutex.Lock()
			cacheKey := fmt.Sprintf("%s_%d", source, cacheID)
			documentCache[cacheKey] = doc
			cacheMutex.Unlock()
		}
		
		if scanErr != nil {
			log.Printf("Error scanning row from %s: %v", source, scanErr)
			continue
		}

		doc.Source = source

		// 根据标题判断文档类型 - 使用更符合实际的判断标准
		if strings.Contains(doc.Title, "刑") {
			doc.Type = "刑事"
		} else if strings.Contains(doc.Title, "民") {
			doc.Type = "民事"
		} else {
			doc.Type = "其他"
		}

		results = append(results, doc)
	}
	
	log.Printf("Found %d results in %s database before type filtering", len(results), source)
	
	// 二次过滤：如果指定了案件类型，确保只返回匹配的文档
	if docType != "全部" {
		var filteredResults []Document
		for _, doc := range results {
			if doc.Type == docType {
				filteredResults = append(filteredResults, doc)
			}
		}
		log.Printf("Filtered to %d results of type '%s' in %s database", len(filteredResults), docType, source)
		return filteredResults, nil
	}
	
	return results, nil
}

// getTableColumns returns a list of column names for a given table
func getTableColumns(db *sql.DB, tableName string) ([]string, error) {
	// PRAGMA table_info is a SQLite-specific command to get table structure
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name string
		var type_ string
		var notnull int
		var dflt_value interface{}
		var pk int

		// SQLite PRAGMA table_info returns: cid, name, type, notnull, dflt_value, pk
		err := rows.Scan(&cid, &name, &type_, &notnull, &dflt_value, &pk)
		if err != nil {
			return nil, err
		}

		columns = append(columns, name)
	}

	return columns, nil
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getDocument retrieves a single document by ID
func getDocument(c *gin.Context) {
	id := c.Param("id")
	source := c.Query("source")
	query := c.Query("q") // 获取搜索关键词参数，用于高亮显示
	
	log.Printf("Getting document with ID: %s from source: %s, search query: %s", id, source, query)
	
	// 先检查缓存中是否存在文档，这对于没有ID列的数据库很重要
	cacheKey := fmt.Sprintf("%s_%s", source, id)
	cacheMutex.Lock()
	if doc, found := documentCache[cacheKey]; found {
		cacheMutex.Unlock()
		log.Printf("Found document in cache with key: %s", cacheKey)
		
		// 返回缓存中的文档
		response := gin.H{
			"id":      doc.ID,
			"title":   doc.Title,
			"content": doc.Content,
			"source":  doc.Source,
			"type":    doc.Type,
			"query":   query,
		}
		
		c.JSON(http.StatusOK, response)
		return
	}
	cacheMutex.Unlock()
	
	var db *sql.DB
	
	// Select the appropriate database
	switch source {
	case "台州中院":
		db = tzDB
	case "温岭法院":
		db = wlDB
	case "已导入数据":
		db = userDB
	default:
		log.Printf("Invalid source: %s", source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source"})
		return
	}
	
	// First, check the table structure to see if we have an id column
	columns, err := getTableColumns(db, "documents")
	if err != nil {
		log.Printf("Error getting table columns for %s: %v", source, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	
	hasIDColumn := contains(columns, "id")
	log.Printf("Database %s has ID column: %v", source, hasIDColumn)
	
	var doc Document
	
	if hasIDColumn {
		// If the table has an ID column, query by ID
		err = db.QueryRow("SELECT id, title, content FROM documents WHERE id = ?", id).Scan(&doc.ID, &doc.Title, &doc.Content)
		if err != nil {
			log.Printf("Error querying document with ID %s from %s: %v", id, source, err)
			c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
			return
		}
	} else {
		// For databases without ID column, we'll need to find the document by position
		// since we used the row number as ID in the search results
		// This approach is inefficient but necessary without an ID column
		
		// 1. Convert the string ID to int
		rowNum, err := strconv.Atoi(id)
		if err != nil {
			log.Printf("Invalid ID format: %s", id)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
			return
		}
		
			// 对于没有ID列的数据库，采用更稳定的方法：使用title和content匹配来查找文档
		// 构建包含全部文档的切片
		documents, err := getAllDocuments(db, source)
		if err != nil {
			log.Printf("Error getting all documents from %s: %v", source, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		
		// 检查行号是否在有效范围内
		if rowNum <= 0 || rowNum > len(documents) {
			log.Printf("Invalid row number %d (total documents: %d)", rowNum, len(documents))
			c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
			return
		}
		
		// 获取指定行号的文档
		doc = documents[rowNum-1] // 行号从1开始，切片索引从0开始
		found := true
		
		// 将获取到的文档存入缓存，以便后续访问
		cacheMutex.Lock()
		cacheKey := fmt.Sprintf("%s_%d", source, rowNum)
		documentCache[cacheKey] = doc
		cacheMutex.Unlock()
		
		if !found {
			log.Printf("Document with position %d not found in %s", rowNum, source)
			c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
			return
		}
	}
	
	// 如果doc.Source为空（说明是通过ID查询到的文档），设置Source字段
	if doc.Source == "" {
		doc.Source = source
	}
	
	// 如果doc.Type为空，根据标题确定文档类型
	if doc.Type == "" {
		if strings.Contains(doc.Title, "刑") {
			doc.Type = "刑事"
		} else if strings.Contains(doc.Title, "民") {
			doc.Type = "民事"
		} else {
			doc.Type = "其他"
		}
	}
	
	// 添加搜索关键词到响应中，便于前端高亮
	response := gin.H{
		"id":      doc.ID,
		"title":   doc.Title,
		"content": doc.Content,
		"source":  doc.Source,
		"type":    doc.Type,
		"query":   query,
	}
	
	log.Printf("Successfully retrieved document: %s", doc.Title)
	c.JSON(http.StatusOK, response)
}

// getAllDocuments 获取指定数据库中的所有文档
func getAllDocuments(db *sql.DB, source string) ([]Document, error) {
	var documents []Document
	
	// 查询所有文档
	rows, err := db.Query("SELECT title, content FROM documents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	// 处理结果
	id := 1 // 为没有ID列的表格生成自增ID
	for rows.Next() {
		var doc Document
		doc.ID = id
		err := rows.Scan(&doc.Title, &doc.Content)
		if err != nil {
			return nil, err
		}
		
		doc.Source = source
		
		// 确定文档类型
		if strings.Contains(doc.Title, "刑") {
			doc.Type = "刑事"
		} else if strings.Contains(doc.Title, "民") {
			doc.Type = "民事"
		} else {
			doc.Type = "其他"
		}
		
		documents = append(documents, doc)
		id++
	}
	
	return documents, nil
}

// uploadDocument handles document uploads
func uploadDocument(c *gin.Context) {
	// 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		log.Printf("上传错误: 未找到文件数据 - %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到上传文件"})
		return
	}
	
	log.Printf("处理上传文件: %s (大小: %.2f MB)", file.Filename, float64(file.Size)/(1024*1024))
	
	// 检查文件扩展名
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".doc" && ext != ".docx" {
		log.Printf("上传错误: 不支持的文件类型 %s", ext)
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 .doc 和 .docx 格式文件"})
		return
	}
	
	// 保存原始文件名作为标题 - 去除扩展名
	originalFileName := strings.TrimSuffix(file.Filename, ext)
	
	// 创建临时文件路径，使用原文件名避免冲突
	// 使用时间戳作为前缀以避免文件名冲突
	timeStamp := time.Now().UnixNano()
	tempFileName := fmt.Sprintf("%d-%s", timeStamp, file.Filename)
	tempFile := filepath.Join("uploads", tempFileName)
	
	// 确保上传目录存在
	err = os.MkdirAll("uploads", 0755)
	if err != nil {
		log.Printf("上传错误: 创建上传目录失败 - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建上传目录失败"})
		return
	}
	
	// 保存上传的文件
	if err := c.SaveUploadedFile(file, tempFile); err != nil {
		log.Printf("上传错误: 保存文件失败 - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败: " + err.Error()})
		return
	}
	
	log.Printf("文件已保存到临时位置: %s", tempFile)
	
	// 从文档中提取内容，但使用原始文件名作为标题
	_, content, err := extractDocumentContent(tempFile)
	if err != nil {
		log.Printf("上传错误: 解析文档内容失败 - %v", err)
		// 清理临时文件
		os.Remove(tempFile)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析文档内容失败: " + err.Error()})
		return
	}
	
	log.Printf("成功提取文档 '%s' 的内容, 将使用原始文件名 '%s' 作为标题, 内容长度: %d", 
		file.Filename, originalFileName, len(content))
	
	// 保存到用户数据库，使用原始文件名作为标题
	_, err = userDB.Exec("INSERT INTO documents (title, content) VALUES (?, ?)", originalFileName, content)
	if err != nil {
		log.Printf("上传错误: 保存到数据库失败 - %v", err)
		// 清理临时文件
		os.Remove(tempFile)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存到数据库失败: " + err.Error()})
		return
	}
	
	log.Printf("文档 '%s' 已成功保存到数据库", file.Filename)
	
	// 清理临时文件
	os.Remove(tempFile)
	log.Printf("临时文件 %s 已删除", tempFile)
	
	c.JSON(http.StatusOK, gin.H{
		"message": "文档上传成功",
		"title": originalFileName,
	})
}

// extractDocumentContent extracts title and content from a Word document
func extractDocumentContent(filePath string) (string, string, error) {
	// Use the ProcessDocument function from document.go
	return ProcessDocument(filePath)
}

// clearUserDatabase 清空用户导入的数据库
func clearUserDatabase(c *gin.Context) {
	log.Println("正在清空用户自定义数据库...")
	
	// 执行DELETE语句清空表中的所有数据
	_, err := userDB.Exec("DELETE FROM documents")
	if err != nil {
		log.Printf("清空数据库失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清空数据库失败: " + err.Error()})
		return
	}
	
	// 清空缓存
	cacheMutex.Lock()
	documentCache = make(map[string]Document)
	cacheMutex.Unlock()
	
	log.Println("用户自定义数据库已成功清空")
	c.JSON(http.StatusOK, gin.H{"message": "数据库已成功清空"})
}
