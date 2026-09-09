allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

val newBuildDir: Directory =
    rootProject.layout.buildDirectory
        .dir("../../build")
        .get()
rootProject.layout.buildDirectory.value(newBuildDir)

subprojects {
    val newSubprojectBuildDir: Directory = newBuildDir.dir(project.name)
    project.layout.buildDirectory.value(newSubprojectBuildDir)
}
subprojects {
    // Plugins are published against whatever SDK was current when they
    // shipped — permission_handler_android asks for 37 — and the build fails
    // outright on a machine that has 36. Pinning every subproject to one SDK
    // keeps the toolchain requirement to a single number, which is also the
    // number CI has to install.
    //
    // Registered before evaluationDependsOn, because that call evaluates the
    // project there and then and an afterEvaluate added afterwards never runs.
    afterEvaluate {
        val android = extensions.findByName("android")
        if (android is com.android.build.gradle.BaseExtension) {
            android.compileSdkVersion(36)
        }
    }
    project.evaluationDependsOn(":app")
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
