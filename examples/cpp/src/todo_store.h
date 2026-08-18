#pragma once

#include <chrono>
#include <cstdint>
#include <iostream>
#include <memory>
#include <mutex>
#include <sstream>
#include <string>
#include <unordered_map>
#include <vector>

namespace todo {
namespace v1 {

struct TodoItem {
    std::string name;
    std::string title;
    std::string description;
    bool completed = false;
    std::string priority;
    int64_t create_time = 0;
    int64_t update_time = 0;
};

class TodoStore {
public:
    static std::shared_ptr<TodoStore> instance() {
        static auto store = std::make_shared<TodoStore>();
        return store;
    }

    TodoItem create(const std::string& parent, const std::string& todo_id,
                    const std::string& title, const std::string& description,
                    bool completed, const std::string& priority) {
        std::string name = parent + "/todos/" + todo_id;
        auto now = now_epoch();
        TodoItem item{name, title, description, completed,
                      priority.empty() ? "PRIORITY_UNSPECIFIED" : priority,
                      now, now};
        std::lock_guard<std::mutex> lock(mu_);
        todos_[name] = item;
        std::cerr << "Created todo: " << name << std::endl;
        std::cerr << "Todo item: " << name << std::endl;
        std::cerr << "Todo item: " << title << std::endl;
        std::cerr << "Todo item: " << description << std::endl;
        std::cerr << "Todo item: " << completed << std::endl;
        std::cerr << "Todo item: " << priority << std::endl;
        std::cerr << "Todo item: " << now << std::endl;
        return item;
    }

    const TodoItem* get(const std::string& name) const {
        std::lock_guard<std::mutex> lock(mu_);
        auto it = todos_.find(name);
        return it != todos_.end() ? &it->second : nullptr;
    }

    std::vector<TodoItem> list() const {
        std::lock_guard<std::mutex> lock(mu_);
        std::vector<TodoItem> out;
        out.reserve(todos_.size());
        for (const auto& [_, v] : todos_) out.push_back(v);
        return out;
    }

    TodoItem* update(const std::string& name) {
        std::lock_guard<std::mutex> lock(mu_);
        auto it = todos_.find(name);
        if (it == todos_.end()) return nullptr;
        it->second.update_time = now_epoch();
        return &it->second;
    }

    bool remove(const std::string& name) {
        std::lock_guard<std::mutex> lock(mu_);
        return todos_.erase(name) > 0;
    }

    static std::string escape_json(const std::string& s) {
        std::string out;
        out.reserve(s.size());
        for (char c : s) {
            switch (c) {
                case '"':  out += "\\\""; break;
                case '\\': out += "\\\\"; break;
                case '\n': out += "\\n";  break;
                case '\r': out += "\\r";  break;
                case '\t': out += "\\t";  break;
                default:   out += c;
            }
        }
        return out;
    }

    static std::string item_to_json(const TodoItem& t) {
        std::ostringstream os;
        os << "{"
           << "\"name\":\"" << escape_json(t.name) << "\","
           << "\"title\":\"" << escape_json(t.title) << "\","
           << "\"description\":\"" << escape_json(t.description) << "\","
           << "\"completed\":" << (t.completed ? "true" : "false") << ","
           << "\"priority\":\"" << escape_json(t.priority) << "\","
           << "\"create_time\":\"" << t.create_time << "Z\","
           << "\"update_time\":\"" << t.update_time << "Z\""
           << "}";
        return os.str();
    }

    static std::string error_json(const std::string& msg) {
        return "{\"error\":\"" + escape_json(msg) + "\"}";
    }

private:
    static int64_t now_epoch() {
        return std::chrono::duration_cast<std::chrono::seconds>(
                   std::chrono::system_clock::now().time_since_epoch())
            .count();
    }

    mutable std::mutex mu_;
    std::unordered_map<std::string, TodoItem> todos_;
};

}  // namespace v1
}  // namespace todo
