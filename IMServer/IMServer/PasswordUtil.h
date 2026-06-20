#pragma once
// 密码加盐哈希工具（纯 C++ 自包含实现，无外部依赖）。
// 存储格式: "v1$<saltHex>$<hashHex>"，hash = 迭代 SHA-256(saltHex+plain)。
// 兼容旧明文密码：VerifyPassword 检测到非 "v1$" 前缀时按明文比对，
// 因此存量账号不受影响，新注册/改密则存哈希。
#include <string>
#include <cstdint>
#include <random>

namespace pwd {

namespace detail {

inline uint32_t rotr(uint32_t x, uint32_t n) { return (x >> n) | (x << (32 - n)); }

// 返回 32 字节原始 SHA-256 摘要
inline std::string sha256_raw(const std::string& msg) {
    static const uint32_t k[64] = {
        0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,
        0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,
        0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
        0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,
        0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,
        0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
        0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,
        0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2
    };
    uint32_t h[8] = {
        0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19
    };

    std::string data = msg;
    uint64_t bitlen = (uint64_t)data.size() * 8;
    data.push_back((char)0x80);
    while (data.size() % 64 != 56) data.push_back((char)0x00);
    for (int i = 7; i >= 0; --i) data.push_back((char)((bitlen >> (i * 8)) & 0xff));

    for (size_t chunk = 0; chunk < data.size(); chunk += 64) {
        uint32_t w[64];
        for (int i = 0; i < 16; ++i) {
            w[i] = ((uint32_t)(uint8_t)data[chunk + i * 4] << 24)
                 | ((uint32_t)(uint8_t)data[chunk + i * 4 + 1] << 16)
                 | ((uint32_t)(uint8_t)data[chunk + i * 4 + 2] << 8)
                 | ((uint32_t)(uint8_t)data[chunk + i * 4 + 3]);
        }
        for (int i = 16; i < 64; ++i) {
            uint32_t s0 = rotr(w[i - 15], 7) ^ rotr(w[i - 15], 18) ^ (w[i - 15] >> 3);
            uint32_t s1 = rotr(w[i - 2], 17) ^ rotr(w[i - 2], 19) ^ (w[i - 2] >> 10);
            w[i] = w[i - 16] + s0 + w[i - 7] + s1;
        }
        uint32_t a = h[0], b = h[1], c = h[2], d = h[3], e = h[4], f = h[5], g = h[6], hh = h[7];
        for (int i = 0; i < 64; ++i) {
            uint32_t S1 = rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25);
            uint32_t ch = (e & f) ^ ((~e) & g);
            uint32_t t1 = hh + S1 + ch + k[i] + w[i];
            uint32_t S0 = rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22);
            uint32_t maj = (a & b) ^ (a & c) ^ (b & c);
            uint32_t t2 = S0 + maj;
            hh = g; g = f; f = e; e = d + t1; d = c; c = b; b = a; a = t1 + t2;
        }
        h[0] += a; h[1] += b; h[2] += c; h[3] += d; h[4] += e; h[5] += f; h[6] += g; h[7] += hh;
    }

    std::string out(32, '\0');
    for (int i = 0; i < 8; ++i) {
        out[i * 4]     = (char)((h[i] >> 24) & 0xff);
        out[i * 4 + 1] = (char)((h[i] >> 16) & 0xff);
        out[i * 4 + 2] = (char)((h[i] >> 8) & 0xff);
        out[i * 4 + 3] = (char)(h[i] & 0xff);
    }
    return out;
}

inline std::string to_hex(const std::string& raw) {
    static const char* d = "0123456789abcdef";
    std::string out;
    out.reserve(raw.size() * 2);
    for (unsigned char c : raw) {
        out.push_back(d[c >> 4]);
        out.push_back(d[c & 0xf]);
    }
    return out;
}

} // namespace detail

inline std::string sha256_hex(const std::string& s) {
    return detail::to_hex(detail::sha256_raw(s));
}

// 迭代次数：增加暴力破解成本（拉伸）。
static const int kIterations = 10000;

// 生成加盐哈希："v1$saltHex$hash"
inline std::string HashPassword(const std::string& plain) {
    std::random_device rd;
    std::string salt(16, '\0');
    for (int i = 0; i < 16; ++i) salt[i] = (char)(rd() & 0xff);
    std::string saltHex = detail::to_hex(salt);

    std::string h = sha256_hex(saltHex + plain);
    for (int i = 1; i < kIterations; ++i) h = sha256_hex(h);
    return std::string("v1$") + saltHex + "$" + h;
}

// 是否为本工具生成的哈希格式
inline bool IsHashed(const std::string& stored) {
    return stored.size() > 3 && stored.compare(0, 3, "v1$") == 0;
}

// 校验明文是否匹配存储值；非哈希格式按旧明文比对（向后兼容）
inline bool VerifyPassword(const std::string& plain, const std::string& stored) {
    if (!IsHashed(stored)) {
        return plain == stored;
    }
    size_t p1 = 3;
    size_t p2 = stored.find('$', p1);
    if (p2 == std::string::npos) return false;
    std::string saltHex = stored.substr(p1, p2 - p1);
    std::string expect = stored.substr(p2 + 1);

    std::string h = sha256_hex(saltHex + plain);
    for (int i = 1; i < kIterations; ++i) h = sha256_hex(h);
    return h == expect;
}

} // namespace pwd
