#include "nu54_siwe.h"
#include "nu54_keccak.h"

#include <stdbool.h>
#include <stdio.h>
#include <string.h>

struct field { const uint8_t *data; size_t size; };

static bool text(struct field field, const char *value)
{
	size_t length = strlen(value);
	return field.size == length && !memcmp(field.data, value, length);
}

static bool prefix(struct field *field, const char *value)
{
	size_t length = strlen(value);
	if (field->size < length || memcmp(field->data, value, length)) return false;
	field->data += length;
	field->size -= length;
	return true;
}

static bool host_byte(uint8_t value)
{
	return (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') ||
	       value == '-' || value == '.' || value == ':';
}

static int hex(uint8_t value)
{
	if (value >= '0' && value <= '9') return value - '0';
	if (value >= 'a' && value <= 'f') return value - 'a' + 10;
	if (value >= 'A' && value <= 'F') return value - 'A' + 10;
	return -1;
}

static int digits(const uint8_t *text, size_t length)
{
	int result = 0;
	for (size_t i = 0; i < length; i++) {
		if (text[i] < '0' || text[i] > '9') return -1;
		result = result * 10 + text[i] - '0';
	}
	return result;
}

static int epoch(struct field time, uint64_t *out)
{
	static const uint8_t months[] = {31,28,31,30,31,30,31,31,30,31,30,31};
	const uint8_t *raw = time.data;
	if ((time.size != 20 && time.size != 24) || raw[4] != '-' || raw[7] != '-' || raw[10] != 'T' ||
	    raw[13] != ':' || raw[16] != ':' || (time.size == 20 ? raw[19] != 'Z' : memcmp(raw + 19,".000Z",5)))
		return -1;
	int year = digits(raw,4), month = digits(raw+5,2), day = digits(raw+8,2);
	int hours = digits(raw+11,2), minutes = digits(raw+14,2), seconds = digits(raw+17,2);
	if (year < 1970 || year > 2099 || month < 1 || month > 12 || day < 1 ||
	    day > months[month-1] + (year % 4 == 0 && month == 2) ||
	    hours < 0 || hours > 23 || minutes < 0 || minutes > 59 || seconds < 0 || seconds > 59) return -1;
	uint64_t days = (uint64_t)(365 * (year - 1970) + (year - 1969) / 4);
	for (int i = 1; i < month; i++) days += months[i-1] + (year % 4 == 0 && i == 2);
	*out = (days + (uint64_t)(day - 1)) * 86400 + (uint64_t)(hours * 3600 + minutes * 60 + seconds);
	return 0;
}

int nu54_siwe_digest(const uint8_t *message, size_t length, const uint8_t *host, size_t host_size,
		      const uint8_t address[20], uint64_t chain, uint64_t now, uint64_t expiry, uint8_t digest[32])
{
	if (!length || length > NU54_SIWE_MAX || !host_size || host_size > 96 ||
	    (chain != 11155111 && chain != 84532 && chain != 8283) || expiry <= now || expiry - now > 120) return -1;
	for (size_t i = 0; i < host_size; i++) if (!host_byte(host[i])) return -1;
	struct field fields[10] = {{0}};
	size_t row = 0, start = 0;
	for (size_t i = 0; i < length; i++) {
		uint8_t value = message[i];
		if (value == '\n') {
			if (row >= 9) return -1;
			fields[row++] = (struct field){message + start,i - start};
			start = i + 1;
		} else if (value < 0x20 || value > 0x7e) return -1;
	}
	if (row != 9) return -1;
	fields[9] = (struct field){message + start,length - start};
	if (fields[0].size < host_size || memcmp(fields[0].data,host,host_size) ||
	    !text((struct field){fields[0].data + host_size,fields[0].size - host_size},
		  " wants you to sign in with your Ethereum account:") ||
	    !prefix(&fields[1],"0x") || fields[1].size != 40 || fields[2].size || fields[3].size ||
	    !prefix(&fields[4],"URI: https://") || fields[4].size <= host_size ||
	    memcmp(fields[4].data,host,host_size) || fields[4].data[host_size] != '/' ||
	    !text(fields[5],"Version: 1") || !prefix(&fields[6],"Chain ID: ") ||
	    fields[6].size > 8 || digits(fields[6].data,fields[6].size) != (int)chain ||
	    !prefix(&fields[7],"Nonce: ") || fields[7].size < 8 || fields[7].size > 32 ||
	    !prefix(&fields[8],"Issued At: ") || !prefix(&fields[9],"Expiration Time: ")) return -1;
	for (size_t i = 0; i < 20; i++) {
		int a = hex(fields[1].data[i*2]), b = hex(fields[1].data[i*2+1]);
		if (a < 0 || b < 0 || (a*16+b) != address[i]) return -1;
	}
	for (size_t i = 0; i < fields[7].size; i++) {
		uint8_t c = fields[7].data[i];
		if (!((c>='A' && c<='Z') || (c>='a' && c<='z') || (c>='0' && c<='9'))) return -1;
	}
	uint64_t issued, expires;
	if (epoch(fields[8],&issued) || epoch(fields[9],&expires) || issued > now ||
	    expires != expiry || expiry - issued > 120) return -1;
	uint8_t bytes[NU54_SIWE_MAX + 40];
	int prefix_length = snprintf((char *)bytes,40,"\x19" "Ethereum Signed Message:\n%zu",length);
	if (prefix_length < 1 || prefix_length >= 40) return -1;
	memcpy(bytes + prefix_length,message,length);
	nu54_keccak256(bytes,(size_t)prefix_length + length,digest);
	return 0;
}
