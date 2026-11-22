import java.io.OutputStream;
import java.nio.file.Path;
import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.util.HashSet;
import java.util.HexFormat;
import java.util.Locale;
import java.util.jar.JarFile;
import javax.naming.ldap.LdapName;

/** Verifies the signed payload of an Android App Bundle and prints its leaf fingerprint. */
class BundleSigner {
    public static void main(String[] args) throws Exception {
        X509Certificate signer = null;
        var names = new HashSet<String>();
        try (var bundle = new JarFile(Path.of(args[0]).toFile(), true)) {
            var entries = bundle.entries();
            while (entries.hasMoreElements()) {
                var entry = entries.nextElement();
                var name = entry.getName();
                if (!names.add(name)) {
                    throw new SecurityException("Duplicate bundle entry: " + name);
                }
                if (entry.isDirectory() || isSignatureMetadata(name)) {
                    continue;
                }
                // JarFile only verifies the signature after the entire entry has been read.
                try (var input = bundle.getInputStream(entry)) {
                    input.transferTo(OutputStream.nullOutputStream());
                }
                var signers = entry.getCodeSigners();
                if (signers == null || signers.length != 1) {
                    throw new SecurityException("Expected one signer for bundle entry: " + name);
                }
                var certificate = (X509Certificate) signers[0].getSignerCertPath().getCertificates().get(0);
                if (signer != null && !signer.equals(certificate)) {
                    throw new SecurityException("Different signers in bundle payload");
                }
                signer = certificate;
            }
        }
        if (signer == null || !names.contains("BundleConfig.pb") || !names.contains("base/manifest/AndroidManifest.xml")) {
            throw new SecurityException("Missing signed Android App Bundle payload");
        }
        signer.checkValidity();
        for (var component : new LdapName(signer.getSubjectX500Principal().getName()).getRdns()) {
            if ("CN".equalsIgnoreCase(component.getType()) && "Android Debug".equalsIgnoreCase(component.getValue().toString())) {
                throw new SecurityException("Android debug certificate");
            }
        }
        var digest = MessageDigest.getInstance("SHA-256").digest(signer.getEncoded());
        System.out.println(HexFormat.of().formatHex(digest));
    }

    private static boolean isSignatureMetadata(String name) {
        return name.toUpperCase(Locale.ROOT).matches("META-INF/(MANIFEST\\.MF|[^/]+\\.(SF|RSA|DSA|EC)|SIG-[^/]+)");
    }
}
