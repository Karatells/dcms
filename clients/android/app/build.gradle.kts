plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val cfgServerUrl = (findProperty("serverUrl") as String?)?.trim()?.trimEnd('/') ?: "https://dcms.home"
val cfgAppName = (findProperty("appName") as String?)?.trim() ?: "DCMS Social"
val cfgAppId = (findProperty("appId") as String?)?.trim() ?: "org.dcms.app"
val cfgInsecureTls = ((findProperty("allowInsecureTls") as String?) ?: "false").equals("true", ignoreCase = true)
val cfgVersionName = (findProperty("versionName") as String?)?.trim() ?: "1.0.0"
val cfgVersionCode = (findProperty("versionCode") as String?)?.toIntOrNull() ?: 1

val ksPath = (findProperty("keystorePath") as String?)?.trim() ?: ""
val ksPass = (findProperty("keystorePass") as String?) ?: "dcms123456"
val ksAlias = (findProperty("keyAlias") as String?) ?: "dcms"
val ksKeyPass = (findProperty("keyPass") as String?) ?: ksPass

fun escJava(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")

android {
    namespace = "org.dcms.app"
    compileSdk = 34

    defaultConfig {
        applicationId = cfgAppId
        minSdk = 24
        targetSdk = 34
        versionCode = cfgVersionCode
        versionName = cfgVersionName

        resValue("string", "app_name", cfgAppName)
        buildConfigField("String", "SERVER_URL", "\"${escJava(cfgServerUrl)}\"")
        buildConfigField("String", "APP_NAME", "\"${escJava(cfgAppName)}\"")
        buildConfigField("boolean", "ALLOW_INSECURE_TLS", cfgInsecureTls.toString())
    }

    signingConfigs {
        if (ksPath.isNotEmpty() && file(ksPath).exists()) {
            create("release") {
                storeFile = file(ksPath)
                storePassword = ksPass
                keyAlias = ksAlias
                keyPassword = ksKeyPass
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = if (ksPath.isNotEmpty() && file(ksPath).exists()) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
        }
    }

    buildFeatures {
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.12.0")
    implementation("androidx.appcompat:appcompat:1.6.1")
    implementation("androidx.swiperefreshlayout:swiperefreshlayout:1.1.0")
}
