package com.nu54.protocol

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import org.json.JSONObject
import java.net.Socket
import java.net.URI
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.MessageDigest
import java.security.Principal
import java.security.PrivateKey
import java.security.cert.CertificateException
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.security.spec.ECGenParameterSpec
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLEngine
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509ExtendedKeyManager
import javax.net.ssl.X509TrustManager

/** App-private enrolled P-256 identity and anchored HTTPS transport for C3. */
class CommerceHttp(context: Context, private val slots: Set<String>) {
    private val settings = context.getSharedPreferences("nu54-commerce-enrollment-v2", Context.MODE_PRIVATE)
    private val random = java.security.SecureRandom()

    fun randomBytes(size:Double):String {
        require(size==size.toInt().toDouble() && size.toInt() in 1..64)
        return Base64.encodeToString(ByteArray(size.toInt()).also(random::nextBytes),Base64.NO_WRAP)
    }

    private fun journalName(name:String):String {
        require(name in setOf("commerce-inbox-v2","customer-order-v2","customer-payment-v2","customer-qr-v2","customer-stores-v1","terminal-payment-v2","terminal-mode-request-v2","terminal-mode-applied-v2","terminal-public-menu-v1","travel-pickup-kitchen-v1","subscription-alerts-v2","market-lab-8283-v1")
            || ("terminal" in slots && name in setOf("commerce-nearby-inbox-v2","commerce-denials-v2"))
            || name.matches(Regex("cloud-p03-[a-f0-9]{64}|travel-local-[a-f0-9-]{36}|travel-hold-[a-f0-9]{64}|pickup-v2-[a-f0-9]{64}|print-v1-[a-f0-9]{64}|pickup-legacy-[a-f0-9]{64}|customer-(order|payment|qr|legacy|sessions)-[a-f0-9]{64}")))
        return "journal:$name"
    }
    @Synchronized
    private fun journalKey(create:Boolean):SecretKey {
        val alias="nu54-commerce-journal-v2"
        val store=KeyStore.getInstance("AndroidKeyStore").apply{ load(null) }
        (store.getKey(alias,null) as? SecretKey)?.let { return it }
        require(create)
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES,"AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder(alias,KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setKeySize(256).setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }

    fun loadJournal(name:String):String {
        val setting=journalName(name)
        val sealed=settings.getString(setting,"").orEmpty()
        if(sealed.isEmpty())return ""
        val bytes=Base64.decode(sealed,Base64.NO_WRAP)
        require(bytes.size in 28..400028)
        val cipher=Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE,journalKey(false),GCMParameterSpec(128,bytes.copyOfRange(0,12)))
        cipher.updateAAD(setting.encodeToByteArray())
        return cipher.doFinal(bytes,12,bytes.size-12).decodeToString()
    }

    fun saveJournal(name:String,value:String) {
        val setting=journalName(name)
        if(value.isEmpty()){
            require(settings.edit().remove(setting).commit())
            return
        }
        val clear=value.encodeToByteArray()
        require(clear.size <=400000)
        val cipher=Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE,journalKey(true))
        require(cipher.iv.size==12)
        cipher.updateAAD(setting.encodeToByteArray())
        val sealed=Base64.encodeToString(cipher.iv+cipher.doFinal(clear),Base64.NO_WRAP)
        require(settings.edit().putString(setting,sealed).commit())
    }

    private fun keyName(slot: String): String {
        require(slot in slots)
        return "nu54-commerce-$slot-v2"
    }

    @Synchronized
    private fun key(slot: String): KeyStore.PrivateKeyEntry {
        val name = keyName(slot)
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        if (!store.containsAlias(name)) {
            KeyPairGenerator.getInstance("EC","AndroidKeyStore").apply {
                initialize(KeyGenParameterSpec.Builder(name,KeyProperties.PURPOSE_SIGN)
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    // The TLS provider supplies an already-hashed ECDSA handshake transcript.
                    .setDigests(KeyProperties.DIGEST_NONE,KeyProperties.DIGEST_SHA256,KeyProperties.DIGEST_SHA384,KeyProperties.DIGEST_SHA512)
                    .build())
            }.generateKeyPair()
        }
        return store.getEntry(name,null) as KeyStore.PrivateKeyEntry
    }

    fun publicIdentity(slot: String): String = Base64.encodeToString(key(slot).certificate.publicKey.encoded,Base64.NO_WRAP)

    private fun certificates(profile: JSONObject, slot: String): Triple<X509Certificate,Array<X509Certificate>,PrivateKey> {
        val uri = URI(profile.getString("endpoint"))
        require(uri.scheme == "https" && !uri.host.isNullOrEmpty() && uri.rawUserInfo==null &&
            uri.rawPath==(if(slot=="travel") "/v1/private" else "/v2/commerce") && uri.rawQuery==null && uri.rawFragment==null)
        require(Regex("[0-9a-f]{64}").matches(profile.getString("pin")))
        val scope = profile.getJSONObject("scope")
        require(scope.getString("role")== (if(slot=="travel") "private-user" else slot) && scope.getLong("expiresAt")>System.currentTimeMillis()/1000)
        val factory=CertificateFactory.getInstance("X.509")
        fun parse(raw:String): X509Certificate = factory.generateCertificate(Base64.decode(raw,Base64.NO_WRAP).inputStream()) as X509Certificate
        val chainJSON=profile.getJSONArray("certificates")
        require(chainJSON.length() in 1..4)
        val chain=Array(chainJSON.length()){ index -> parse(chainJSON.getString(index)) }
        val identity=key(slot)
        require(chain[0].publicKey.encoded.contentEquals(identity.certificate.publicKey.encoded))
        chain[0].checkValidity()
        return Triple(parse(profile.getString("ca")),chain,identity.privateKey)
    }

    fun configure(slot:String, raw:String) {
        require(raw.toByteArray().size in 1..16_384)
        certificates(JSONObject(raw),slot)
        require(settings.edit().putString(keyName(slot),raw).commit())
    }

    fun load(slot:String):String = settings.getString(keyName(slot),"") ?: ""

    fun exchange(slot:String, envelope:String):String {
        require(slot!="travel")
        return exchangeAt(slot,"POST","",envelope,8192)
    }

    fun exchangeScoped(slot: String, profileDigest: String, envelope: String): String {
        require(slot != "travel" && Regex("[a-f0-9]{64}").matches(profileDigest))
        return exchangeAt(slot, "POST", "", envelope, 8192, profileDigest)
    }

    fun exchangePrint(operation:String, profileDigest:String, reference:String, envelope:String):String {
        val id=Regex("[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}")
        require(Regex("[a-f0-9]{64}").matches(profileDigest))
        val branch=JSONObject(load("operator")).getJSONObject("scope").getString("branchRef")
        require(id.matches(branch) && (id.matches(reference) || (operation=="queue" && reference.isEmpty())))
        val root="/v2/fulfillment/"
        val path=when(operation) {
            "queue" -> root+"print-attempts?branchId="+branch+(if(reference.isEmpty()) "" else "&saleRef="+reference)
            "attempts" -> root+"print-attempts?branchId="+branch+"&receiptRef="+reference
            "decision" -> {
                val body=JSONObject(envelope)
                require(body.getString("branchId")==branch && body.getString("attemptId")==reference)
                root+"print-decisions"
            }
            else -> error("unknown print operation")
        }
        return exchangeAt("operator",if(operation=="decision") "POST" else "GET",path,envelope,4096,profileDigest)
    }

    fun exchangeRenterV1Origin(operation:String, profileDigest:String, body:String):String {
        require(operation in setOf("reserve","confirm") && Regex("[a-f0-9]{64}").matches(profileDigest))
        return exchangeAt("terminal","POST","/v2/private/renter-v1-orders/"+operation,body,2048,profileDigest)
    }

    fun exchangeRenterReturn(profileDigest:String, assignment:String, account:String):String {
        require(Regex("[a-f0-9]{64}").matches(profileDigest) &&
            Regex("[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}").matches(assignment))
        val path="/v2/renter/returns/"+assignment
        if(account.isEmpty()) return exchangeAt("customer","GET",path+"?credential=1&slot=1","",1024,profileDigest)
        require(Regex("0x[a-f0-9]{40}").matches(account))
        return exchangeAt("customer","POST",path+"/accept",JSONObject().put("account",account).toString(),1024,profileDigest)
    }

    fun exchangeTravel(method:String, path:String, envelope:String):String {
        require(method in setOf("GET","POST") && path.matches(Regex(
            "/(?:consents|records(?:/[a-f0-9-]{36})?|visits|reviews|transcriptions|erasure(?:/[a-f0-9-]{36})?|itineraries/(?:preview|apply|generate|[a-f0-9-]{36}))")))
        return exchangeAt("travel",method,path,envelope,1_400_000)
    }

    /** Server trust for one handshake: the chain must end at `ca`, and the leaf must carry the pinned key. System roots are never consulted. */
    private fun anchoredTrust(ca:X509Certificate, pin:String):X509TrustManager {
        val roots=KeyStore.getInstance(KeyStore.getDefaultType()).apply {
            load(null)
            setCertificateEntry("commerce-root",ca)
        }
        val trustFactory=TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(roots) }
        val anchored=trustFactory.trustManagers.filterIsInstance<X509TrustManager>().single()
        return object:X509TrustManager {
            override fun getAcceptedIssuers()=anchored.acceptedIssuers
            override fun checkClientTrusted(chain:Array<out X509Certificate>?, authType:String?) { throw CertificateException("wrong TLS role") }
            override fun checkServerTrusted(chain:Array<out X509Certificate>?,authType:String?) {
                anchored.checkServerTrusted(chain,authType)
                val leaf=chain?.firstOrNull() ?: throw CertificateException("missing server leaf")
                val hash=MessageDigest.getInstance("SHA-256").digest(leaf.publicKey.encoded).joinToString(""){"%02x".format(it)}
                if (hash!=pin) throw CertificateException("wrong commerce pin")
                // The post-handshake HTTPS hostname verifier is also mandatory.
                // The exact pinned SPKI establishes the peer before mTLS sends credentials.
            }
        }
    }

    private fun readCapped(stream:java.io.InputStream, limit:Int):String = stream.use {
        val input=java.io.ByteArrayOutputStream()
        val buffer=ByteArray(4096)
        while (true) {
            val size=it.read(buffer)
            if(size<0) break
            require(input.size()+size<=limit)
            input.write(buffer,0,size)
        }
        input.toString("UTF-8")
    }

    /**
     * One request to the customer entrance of the store server, which asks for no client certificate.
     * The server is accepted only by the trust anchor of this build: `anchor` is `{"origin","ca","pin"}` from the
     * app's deployment configuration, the chain must end at that CA and the leaf must carry that pinned key. No
     * enrollment profile is read and system roots are never consulted. No key manager holds a key, so no client
     * certificate can be offered. `session` is the customer session token of `/v2/customer/commerce` and empty
     * elsewhere; the store list, a store page, a menu page and a menu photo part are read with GET and an empty body.
     * Answers `{"status":n,"body":"…"}`.
     */
    fun customerEntrance(anchor:String, path:String, session:String, body:String):String {
        val reference="[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}"
        // The public reads: the store list, a store page, a page of its menu and a part of a menu photo.
        val read=path=="/v2/customer/stores" ||
            Regex("/v2/customer/stores/$reference(/menu/(0|[1-9][0-9]{0,4})|/photos/[0-9a-f]{64}/(0|[1-9][0-9]{0,5}))?").matches(path)
        val commerce=path=="/v2/customer/commerce"
        require(read || commerce || path in setOf("/v2/customer/challenge","/v2/customer/proof"))
        val payload=body.encodeToByteArray()
        require(if(read) payload.isEmpty() else payload.size in 2..(if(commerce) 8192 else 1024))
        require(if(commerce) Regex("[0-9a-f]{64}").matches(session) else session.isEmpty())
        require(anchor.encodeToByteArray().size in 1..8192)
        val trust=JSONObject(anchor)
        val base=URI(trust.getString("origin"))
        require(base.scheme=="https" && !base.host.isNullOrEmpty() && base.rawUserInfo==null && base.rawPath.isNullOrEmpty() &&
            base.rawQuery==null && base.rawFragment==null && (base.port==-1 || base.port in 1..65535))
        val pin=trust.getString("pin")
        require(Regex("[0-9a-f]{64}").matches(pin))
        val ca=CertificateFactory.getInstance("X.509")
            .generateCertificate(Base64.decode(trust.getString("ca"),Base64.NO_WRAP).inputStream()) as X509Certificate
        val anonymous=object:X509ExtendedKeyManager() {
            override fun getClientAliases(type:String?, issuers:Array<out Principal>?):Array<String>?=null
            override fun chooseClientAlias(types:Array<out String>?,issuers:Array<out Principal>?,socket:Socket?):String?=null
            override fun chooseEngineClientAlias(types:Array<out String>?,issuers:Array<out Principal>?,engine:SSLEngine?):String?=null
            override fun getServerAliases(type:String?,issuers:Array<out Principal>?):Array<String>?=null
            override fun chooseServerAlias(type:String?,issuers:Array<out Principal>?,socket:Socket?):String?=null
            override fun getPrivateKey(alias:String?):PrivateKey?=null
            override fun getCertificateChain(alias:String?):Array<X509Certificate>?=null
        }
        val tls=SSLContext.getInstance("TLSv1.3").apply { init(arrayOf(anonymous),arrayOf(anchoredTrust(ca,pin)),random) }
        val host=base.host
        val connection=java.net.URL("https",host,base.port,path).openConnection(java.net.Proxy.NO_PROXY) as HttpsURLConnection
        try {
            connection.sslSocketFactory=tls.socketFactory
            connection.hostnameVerifier=javax.net.ssl.HostnameVerifier { name, session ->
                name==host && session.protocol=="TLSv1.3" && HttpsURLConnection.getDefaultHostnameVerifier().verify(name,session)
            }
            connection.doOutput=!read
            connection.instanceFollowRedirects=false
            connection.useCaches=false
            connection.connectTimeout=5000
            connection.readTimeout=7000
            connection.requestMethod=if(read) "GET" else "POST"
            connection.setRequestProperty("Content-Type","application/json")
            connection.setRequestProperty("Cache-Control","no-store")
            if(commerce) connection.setRequestProperty("Authorization","Bearer $session")
            if(!read) connection.outputStream.use { it.write(payload) }
            val status=connection.responseCode
            // What the entrance answers with. A redirect or anything else is not an answer.
            require(status in setOf(200,400,404,409,429,503))
            val stream=if(status==200) connection.inputStream else connection.errorStream
            val limit=if(read || commerce) 32768 else 8192
            return JSONObject().put("status",status).put("body",if(stream==null) "" else readCapped(stream,limit)).toString()
        } finally {
            connection.disconnect()
        }
    }

    private fun exchangeAt(slot:String,method:String,path:String,envelope:String,maxSize:Int,profileDigest:String?=null):String {
        val payload=envelope.encodeToByteArray()
        require(if(method=="GET") payload.isEmpty() else payload.size in 2..maxSize)
        val raw=load(slot)
        if (profileDigest != null) {
            val current=MessageDigest.getInstance("SHA-256").digest(raw.encodeToByteArray())
                .joinToString("") { "%02x".format(it) }
            require(current == profileDigest)
        }
        val profile=JSONObject(raw)
        val (ca,chain,privateKey)=certificates(profile,slot)
        val pin=profile.getString("pin")
        val host=URI(profile.getString("endpoint")).host
        val trust=anchoredTrust(ca,pin)
        val owner=object:X509ExtendedKeyManager() {
            override fun getClientAliases(type:String?, issuers:Array<out Principal>?)=if(type=="EC" || type?.startsWith("EC_")==true) arrayOf(slot) else null
            override fun chooseClientAlias(types:Array<out String>?,issuers:Array<out Principal>?,socket:Socket?)=if(types?.any{it=="EC" || it.startsWith("EC_")}==true) slot else null
            override fun chooseEngineClientAlias(types:Array<out String>?,issuers:Array<out Principal>?,engine:SSLEngine?)=chooseClientAlias(types,issuers,null)
            override fun getServerAliases(type:String?,issuers:Array<out Principal>?)=null
            override fun chooseServerAlias(type:String?,issuers:Array<out Principal>?,socket:Socket?)=null
            override fun getPrivateKey(alias:String?)=if(alias==slot) privateKey else null
            override fun getCertificateChain(alias:String?)=if(alias==slot) chain else null
        }
        val tls=SSLContext.getInstance("TLSv1.3").apply { init(arrayOf(owner),arrayOf(trust),random) }
        val endpoint=URI(profile.getString("endpoint"))
        val address=if(path.startsWith("/v2/fulfillment/") || path.startsWith("/v2/renter/returns/") || path.startsWith("/v2/private/renter-v1-orders/")) endpoint.resolve(path) else URI(endpoint.toString()+path)
        val connection=address.toURL().openConnection(java.net.Proxy.NO_PROXY) as HttpsURLConnection
        try {
            connection.sslSocketFactory=tls.socketFactory
            connection.hostnameVerifier=javax.net.ssl.HostnameVerifier { name, session ->
                name==host && session.protocol=="TLSv1.3" && HttpsURLConnection.getDefaultHostnameVerifier().verify(name,session)
            }
            connection.doOutput=method=="POST"
            connection.instanceFollowRedirects=false
            connection.connectTimeout=5000
            connection.readTimeout=if(path=="/itineraries/generate") 70000 else 7000
            connection.requestMethod=method
            connection.setRequestProperty("Content-Type","application/json")
            connection.setRequestProperty("Cache-Control","no-store")
            if(method=="POST") connection.outputStream.use { it.write(payload) }
            val status=connection.responseCode
            require(status in setOf(200,400,409,503))
            val stream=(if(status==200) connection.inputStream else connection.errorStream) ?: error("missing response")
            return readCapped(stream,32768)
        } finally {
            connection.disconnect()
        }
    }
}
