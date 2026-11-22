#include <jni.h>

extern int nu54_p03_start(char *, void *, int, char *, char *, long long);
extern void nu54_p03_stop(void);
extern int nu54_p03_prepare(char *, void *, int, char *, int);
extern int nu54_p03_install(char *, void *, int, char *, char *, char *);

static void wipe(jbyte *bytes) {
    volatile jbyte *secret = bytes;
    for (int i = 0; i < 32; i++) secret[i] = 0;
}

JNIEXPORT jint JNICALL Java_com_nu54renter_ble_CloudWalletProfileModule_startGoParticipant(
    JNIEnv *env, jobject self, jstring path, jbyteArray key, jstring owner, jstring authority, jlong expires) {
    (void)self;
    if (!path || !owner || !authority || !key || (*env)->GetArrayLength(env, key) != 32) return -1;
    const char *directory = (*env)->GetStringUTFChars(env, path, NULL);
    const char *subject = (*env)->GetStringUTFChars(env, owner, NULL);
    const char *public_key = (*env)->GetStringUTFChars(env, authority, NULL);
    jint result = -1;
    if (directory && subject && public_key) {
        jbyte bytes[32];
        (*env)->GetByteArrayRegion(env, key, 0, 32, bytes);
        if (!(*env)->ExceptionCheck(env)) result = nu54_p03_start((char *)directory, bytes, 32, (char *)subject, (char *)public_key, expires);
        wipe(bytes);
    }
    if (public_key) (*env)->ReleaseStringUTFChars(env, authority, public_key);
    if (subject) (*env)->ReleaseStringUTFChars(env, owner, subject);
    if (directory) (*env)->ReleaseStringUTFChars(env, path, directory);
    return result;
}

JNIEXPORT jstring JNICALL Java_com_nu54renter_ble_CloudWalletProfileModule_prepareGoParticipant(
    JNIEnv *env, jobject self, jstring path, jbyteArray key) {
    (void)self;
    if (!path || !key || (*env)->GetArrayLength(env, key) != 32) return NULL;
    const char *directory = (*env)->GetStringUTFChars(env, path, NULL);
    if (!directory) return NULL;
    jbyte bytes[32];
    char csr[4096];
    (*env)->GetByteArrayRegion(env, key, 0, 32, bytes);
    int result = (*env)->ExceptionCheck(env) ? -1 : nu54_p03_prepare((char *)directory, bytes, 32, csr, sizeof(csr));
    wipe(bytes);
    (*env)->ReleaseStringUTFChars(env, path, directory);
    return result > 0 ? (*env)->NewStringUTF(env, csr) : NULL;
}

JNIEXPORT jint JNICALL Java_com_nu54renter_ble_CloudWalletProfileModule_installGoParticipant(
    JNIEnv *env, jobject self, jstring path, jbyteArray key, jstring owner, jstring authority, jstring roster) {
    (void)self;
    if (!path || !owner || !authority || !roster || !key || (*env)->GetArrayLength(env, key) != 32) return -1;
    const char *directory = (*env)->GetStringUTFChars(env, path, NULL);
    const char *subject = (*env)->GetStringUTFChars(env, owner, NULL);
    const char *public_key = (*env)->GetStringUTFChars(env, authority, NULL);
    const char *envelope = (*env)->GetStringUTFChars(env, roster, NULL);
    jint result = -1;
    if (directory && subject && public_key && envelope) {
        jbyte bytes[32];
        (*env)->GetByteArrayRegion(env, key, 0, 32, bytes);
        if (!(*env)->ExceptionCheck(env)) result = nu54_p03_install((char *)directory, bytes, 32, (char *)subject, (char *)public_key, (char *)envelope);
        wipe(bytes);
    }
    if (envelope) (*env)->ReleaseStringUTFChars(env, roster, envelope);
    if (public_key) (*env)->ReleaseStringUTFChars(env, authority, public_key);
    if (subject) (*env)->ReleaseStringUTFChars(env, owner, subject);
    if (directory) (*env)->ReleaseStringUTFChars(env, path, directory);
    return result;
}


JNIEXPORT void JNICALL Java_com_nu54renter_ble_CloudWalletProfileModule_stopGoParticipant(JNIEnv *env, jobject self) {
    (void)env;
    (void)self;
    nu54_p03_stop();
}
