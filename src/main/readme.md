# Part II: Single-worker word count

这部分比较简单了，part1我们的mapF的value是空的，所以这次的value就是每个文件下每个单词的数目，最后转成键值对数组

```go
func mapF(document string, value string) (res []mapreduce.KeyValue) {
	// TODO: you have to write this function

	words := strings.FieldsFunc(value, func(c rune) bool {
		return !unicode.IsLetter(c)
	})
	mp := make(map[string]int)
	for _, word := range words {
		mp[word]++
	}

	var ans []mapreduce.KeyValue
	for k, v := range mp {
		ans = append(ans, mapreduce.KeyValue{k, strconv.Itoa(v)})
	}

	return ans
}
```
注意split的方式，传统的string.Field()是无法对例如"the,the"进行split的，最终会分成"the,the",而不是"the the"，这点需要注意一下


reduce就是合并value即可
```go
func reduceF(key string, values []string) string {
	// TODO: you also have to write this function
	tot := 0

	for _, value := range values {
		cur, err := strconv.Atoi(value)
		if err != nil {
			log.Fatal()
		}
		tot += cur
	}
	return strconv.Itoa(tot)
}
```
