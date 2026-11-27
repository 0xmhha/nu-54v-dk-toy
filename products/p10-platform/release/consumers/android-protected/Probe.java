package org.nu54.qa.protectedprobe;

import android.app.Instrumentation;
import android.content.Context;
import android.os.Bundle;
import java.io.File;
import java.io.FileInputStream;
import java.security.KeyStore;
import java.security.MessageDigest;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import java.util.Locale;
import org.json.JSONObject;

public final class Probe extends Instrumentation {
    private String stage;

    @Override public void onCreate(Bundle args) {
        super.onCreate(args);
        stage = args.getString("stage");
        start();
    }

    private String digest(byte[] value) throws Exception {
        StringBuilder out = new StringBuilder();
        for (byte b : MessageDigest.getInstance("SHA-256").digest(value)) {
            out.append(String.format(Locale.ROOT, "%02x", b));
        }
        return out.toString();
    }

    @Override public void onStart() {
        Bundle report = new Bundle();
        try {
            Context app = getTargetContext();
            if (!TARGET.equals(app.getPackageName())) throw new IllegalStateException("wrong package");
            Class<?> type = app.getClassLoader().loadClass("com.nu54renter.ble.OwnerKeys");
            Object keys = type.getDeclaredConstructor(Context.class).newInstance(app);
            Method load = type.getMethod("load");
            Method compare = type.getMethod("compareAndSave", String.class, String.class);
            String previous = (String) load.invoke(keys);
            boolean stale = false;
            boolean olderGeneration = false;
            switch (stage) {
                case "seed":
                    if (!previous.isEmpty()) throw new IllegalStateException("unexpected existing profile");
                    type.getMethod("save", String.class).invoke(keys, BEFORE);
                    break;
                case "advance":
                    if (!BEFORE.equals(previous) || !Boolean.TRUE.equals(compare.invoke(keys, BEFORE, LATEST))) {
                        throw new IllegalStateException("pending scope differs");
                    }
                    break;
                case "prove":
                    if (!LATEST.equals(previous)) throw new IllegalStateException("latest profile not retained");
                    stale = Boolean.FALSE.equals(compare.invoke(keys, BEFORE, BEFORE));
                    JSONObject outdated = new JSONObject(BEFORE);
                    JSONObject owner = outdated.getJSONObject("verified");
                    owner.put("generation", owner.getInt("generation") - 1);
                    olderGeneration = Boolean.FALSE.equals(compare.invoke(keys, outdated.toString(), BEFORE));
                    if (!stale || !olderGeneration) throw new IllegalStateException("stale reservation resurrected");
                    break;
                case "inspect": break;
                default: throw new IllegalArgumentException("unknown stage");
            }
            String current = (String) load.invoke(keys);
            if (!BEFORE.equals(current) && !LATEST.equals(current)) throw new IllegalStateException("profile differs");
            JSONObject profile = new JSONObject(current);
            JSONObject owner = profile.getJSONObject("verified");
            String aadScope = owner.getString("boardFingerprint") + ":" + owner.getInt("generation")
                + ":" + owner.getString("network") + ":" + owner.getString("address");
            File encrypted = new File(app.getNoBackupFilesDir(), "nu54-owner-profile.v2");
            byte[] sealed;
            try (FileInputStream input = new FileInputStream(encrypted)) {
                sealed = new byte[(int) encrypted.length()];
                int offset = 0;
                while (offset < sealed.length) {
                    int size = input.read(sealed, offset, sealed.length - offset);
                    if (size < 0) throw new IllegalStateException("short protected profile");
                    offset += size;
                }
            }
            byte[] clear = current.getBytes(StandardCharsets.UTF_8);
            if (sealed.length != clear.length + 28 ||
                    new String(sealed, StandardCharsets.ISO_8859_1).contains(current)) {
                throw new IllegalStateException("invalid AES-GCM envelope");
            }
            String assurance = "owner-hardware-unavailable";
            try {
                KeyStore.PrivateKeyEntry identity = (KeyStore.PrivateKeyEntry) type.getMethod("identity").invoke(keys);
                assurance = "owner-hardware-available";
                report.putString("ownerKey", digest(identity.getCertificate().getPublicKey().getEncoded()));
            } catch (java.lang.reflect.InvocationTargetException error) {
                if (!(error.getCause() instanceof IllegalArgumentException) ||
                    !"hardware-backed owner identity unavailable".equals(error.getCause().getMessage())) {
                    throw error;
                }
            }
            report.putString("result", "PASS");
            report.putString("record", digest(clear));
            report.putString("ciphertext", digest(sealed));
            report.putString("ownerScope", digest(aadScope.getBytes(StandardCharsets.UTF_8)));
            report.putString("phase", profile.has("signing") ? "unresolved" : "pending");
            report.putString("staleRejected", stale ? "true" : "false");
            report.putString("olderGenerationRejected", olderGeneration ? "true" : "false");
            report.putString("assurance", assurance);
            finish(-1, report);
        } catch (Throwable error) {
            report.clear();
            report.putString("result", "FAIL-" + error.getClass().getSimpleName());
            finish(0, report);
        }
    }

    private static final String TARGET = __QA_TARGET__;
    private static final String BEFORE = __C1_BEFORE__;
    private static final String LATEST = __C1_LATEST__;
}
