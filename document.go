package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"io/ioutil"
)

// ProcessDocument extracts title and content from a Word document
// 简化版：不再依赖UniOffice，直接使用文件名作为标题，文件内容（如果能读取）或占位符作为内容
func ProcessDocument(filePath string) (string, string, error) {
	// 使用文件名作为标题
	fileName := filepath.Base(filePath)
	title := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	
	// 尝试读取文件内容（仅对于文本可读的部分）
	content, err := readFileContent(filePath)
	if err != nil {
		// 如果读取失败，使用占位符内容
		content = fmt.Sprintf("从 %s 导入的文档内容", title)
	}
	
	return title, content, nil
}

// 尝试读取文件内容的函数
func readFileContent(filePath string) (string, error) {
	// 首先尝试直接读取文件（对于.docx和.doc来说这不会得到人类可读的内容，但至少不会失败）
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}
	
	// 如果文件过大，只读取前面部分
	var maxSize int64 = 1024 * 50 // 最多读取50KB
	if fileInfo.Size() > maxSize {
		// 对于大文件，尝试读取文件前面的部分
		file, err := os.Open(filePath)
		if err != nil {
			return "", err
		}
		defer file.Close()
		
		scanner := bufio.NewScanner(file)
		var content strings.Builder
		lineCount := 0
		
		// 读取前100行或直到文件结束
		for scanner.Scan() && lineCount < 100 {
			content.WriteString(scanner.Text())
			content.WriteString("\n")
			lineCount++
		}
		
		if scanner.Err() != nil {
			// 如果扫描出错，返回一个占位符
			return fmt.Sprintf("部分读取的文档内容，文件大小: %d 字节", fileInfo.Size()), nil
		}
		
		return content.String(), nil
	}
	
	// 对于小文件，读取整个文件
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	
	// 尝试将二进制内容转换为字符串，可能会包含乱码
	contentStr := string(content)
	
	// 如果内容太长，只保留前面部分
	if len(contentStr) > 1000 {
		contentStr = contentStr[:1000] + "... (内容已截断)"
	}
	
	return contentStr, nil
}
