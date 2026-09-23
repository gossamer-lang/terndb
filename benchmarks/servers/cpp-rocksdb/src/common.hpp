// What the three programs share: the key encoding, the stored row, and the id
// sequence every benchmark target walks.
#pragma once

#include <cstdint>
#include <string>

namespace bench {

// Big-endian, so the store's byte order is the ids' numeric order.
inline std::string key(std::uint64_t id) {
    std::string k(8, '\0');
    for (int i = 7; i >= 0; --i) {
        k[i] = static_cast<char>(id & 0xff);
        id >>= 8;
    }
    return k;
}

// The row as its finished JSON, which is what the store holds and serves - the
// same shape rust-redb stores.
inline std::string row(std::uint64_t id) {
    auto n = std::to_string(id);
    return "{\"id\":" + n + ",\"name\":\"user-" + n + "\",\"email\":\"user-" + n +
           "@example.com\",\"score\":1.5,\"active\":true}";
}

// Every target walks the same id sequence, so each reads the same rows in the
// same order.
constexpr std::int64_t STRIDE = 2654435761;
constexpr std::int64_t BATCH = 1024;

inline std::uint64_t id_at(std::int64_t i, std::int64_t rows) {
    return static_cast<std::uint64_t>(
        static_cast<std::int64_t>(static_cast<std::uint64_t>(i) * static_cast<std::uint64_t>(STRIDE)) % rows + 1);
}

}  // namespace bench
