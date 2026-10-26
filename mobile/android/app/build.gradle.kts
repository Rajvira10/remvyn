import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// Release signing comes from a properties file kept outside the repo:
// REMVYN_SIGNING=/path/to/signing.properties (storeFile, storePassword, keyAlias, keyPassword).
val signing = Properties().apply {
    val path = System.getenv("REMVYN_SIGNING") ?: rootProject.file("signing.properties").path
    val f = file(path)
    if (f.exists()) f.inputStream().use { load(it) }
}

android {
    namespace = "io.github.rajvira10.remvyn"
    compileSdk = 36

    defaultConfig {
        applicationId = "io.github.rajvira10.remvyn"
        minSdk = 26
        targetSdk = 36
        versionCode = 4
        versionName = "2.1.2"
        // Built-in GitHub App, same as the desktop; override with the env vars.
        buildConfigField("String", "GITHUB_CLIENT_ID", "\"${System.getenv("REMVYN_GITHUB_CLIENT_ID") ?: "Iv23liVRmducYJnHWSnT"}\"")
        buildConfigField("String", "GITHUB_APP_SLUG", "\"${System.getenv("REMVYN_GITHUB_APP_SLUG") ?: "remvyn-sync-rajvira10"}\"")
    }

    signingConfigs {
        if (signing.getProperty("storeFile") != null) {
            create("release") {
                storeFile = file(signing.getProperty("storeFile"))
                storePassword = signing.getProperty("storePassword")
                keyAlias = signing.getProperty("keyAlias")
                keyPassword = signing.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.findByName("release") ?: signingConfigs.getByName("debug")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    packaging {
        resources.excludes += setOf("META-INF/versions/9/OSGI-INF/MANIFEST.MF", "META-INF/DEPENDENCIES")
    }
}

kotlin {
    compilerOptions { jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17) }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2025.10.01")
    implementation(composeBom)
    implementation("androidx.core:core-ktx:1.17.0")
    implementation("androidx.activity:activity-compose:1.11.0")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-core")
    implementation("com.github.mwiede:jsch:2.28.8")
    implementation("org.bouncycastle:bcprov-jdk18on:1.79")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}
