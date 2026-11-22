#include "wallet.h"
#include <jni.h>
#include <time.h>

static int copy(JNIEnv *env, jbyteArray input, uint8_t *out, size_t length)
{
    if (!input || (*env)->GetArrayLength(env, input) != (jsize)length) return 0;
    (*env)->GetByteArrayRegion(env, input, 0, (jsize)length, (jbyte *)out);
    return !(*env)->ExceptionCheck(env);
}

static jstring encode(JNIEnv *env, const uint8_t *out, size_t length)
{
    char encoded[2 * LAB_TX_LENGTH + 3] = "0x";
    lab_encode(out, length, encoded + 2);
    return (*env)->NewStringUTF(env, encoded);
}

JNIEXPORT jstring JNICALL Java_com_nu54renter_ble_MarketLabModule_nativeAddress(JNIEnv *env, jobject module,
    jbyteArray key, jbyteArray random)
{
    (void)module;
    uint8_t seed[LAB_KEY_LENGTH], entropy[32], address[20];
    int valid = copy(env, key, seed, sizeof seed) && copy(env, random, entropy, sizeof entropy) &&
        lab_address(seed, entropy, address);
    lab_wipe(seed, sizeof seed);
    lab_wipe(entropy, sizeof entropy);
    return valid ? encode(env, address, sizeof address) : NULL;
}

JNIEXPORT jstring JNICALL Java_com_nu54renter_ble_MarketLabModule_nativeSign(JNIEnv *env, jobject module,
    jbyteArray key, jbyteArray random, jbyteArray domain, jbyteArray to, jlong nonce, jlong gas_price,
    jbyteArray data, jbyteArray quote)
{
    (void)module;
    uint8_t seed[LAB_KEY_LENGTH], entropy[32], scope[LAB_SCOPE_LENGTH], target[20];
    uint8_t calldata[LAB_QUOTE_LENGTH], offer[LAB_QUOTE_LENGTH], raw[LAB_TX_LENGTH];
    size_t data_length = data ? (size_t)(*env)->GetArrayLength(env, data) : 0;
    size_t raw_length = 0;
    time_t now = time(NULL);
    int valid = now > 0 && nonce >= 0 && gas_price > 0 && (data_length == 68 || data_length == LAB_BOUND_APPROVE_LENGTH || data_length == LAB_QUOTE_LENGTH) &&
        copy(env, domain, scope, sizeof scope) && copy(env, to, target, sizeof target) &&
        copy(env, data, calldata, data_length) && copy(env, quote, offer, sizeof offer) &&
        copy(env, key, seed, sizeof seed) && copy(env, random, entropy, sizeof entropy) &&
        lab_sign(seed, entropy, scope, target, (uint64_t)nonce, (uint64_t)gas_price, (uint64_t)now,
            calldata, data_length, offer, raw, &raw_length);
    lab_wipe(seed, sizeof seed);
    lab_wipe(entropy, sizeof entropy);
    return valid ? encode(env, raw, raw_length) : NULL;
}
