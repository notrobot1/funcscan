#include <iostream>
#include <stdexcept>
#include <string>

std::string encode(const std::string& s);
std::string sanitize(const std::string& data);

std::string loadFile(const std::string& path) {
    return "data from " + path;
}

std::string sanitize(const std::string& data) {
    return data;
}

std::string validate(const std::string& data) {
    if (data.empty()) throw std::runtime_error("empty data");
    return sanitize(data);
}

std::string encode(const std::string& s) {
    return "[" + s + "]";
}

std::string transform(const std::string& data) {
    return encode(data);
}

std::string runPipeline(const std::string& path) {
    std::string raw = loadFile(path);
    std::string validated = validate(raw);
    return transform(validated);
}

int main() {
    try {
        std::string result = runPipeline("input.csv");
        std::cout << "result: " << result << std::endl;
    } catch (const std::exception& e) {
        std::cerr << "error: " << e.what() << std::endl;
        return 1;
    }
    return 0;
}
