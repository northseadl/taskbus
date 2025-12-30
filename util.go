package taskbus

import "strings"

// trimTopicPrefix 从 topic 中移除指定前缀，返回纯业务名称。
// 若 topic 不以 prefix 开头，则原样返回。
func trimTopicPrefix(topic, prefix string) string {
	if strings.HasPrefix(topic, prefix) {
		return topic[len(prefix):]
	}
	return topic
}

// buildTopicPrefix 构建带命名空间的 topic 前缀。
// 例如：buildTopicPrefix("myns", "job") => "taskbus.myns.job."
func buildTopicPrefix(namespace, component string) string {
	return "taskbus." + namespace + "." + component + "."
}

// buildTopic 构建完整的 topic 名称。
// 例如：buildTopic("myns", "job", "example.echo") => "taskbus.myns.job.example.echo"
func buildTopic(namespace, component, name string) string {
	return buildTopicPrefix(namespace, component) + name
}

// buildWildcardTopic 构建通配符 topic（用于订阅）。
// 例如：buildWildcardTopic("myns", "job") => "taskbus.myns.job.#"
func buildWildcardTopic(namespace, component string) string {
	return buildTopicPrefix(namespace, component) + "#"
}
