// Exact aliases for labels created by earlier releases. User-authored text is preserved.
const labels = new Map([
  ["新对话", "New conversation"],
  ["[内置] 删除类接口路径", "[Built-in] Deletion endpoint paths"],
]);

export function englishSystemLabel(value) {
  return labels.get(value) ?? value;
}
