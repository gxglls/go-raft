// Powered by Infostretch 

timestamps {

node () {

	stage ('shell_cmd - Checkout') {
 	 checkout([$class: 'GitSCM', branches: [[name: '*/master']], doGenerateSubmoduleConfigurations: false, extensions: [], submoduleCfg: [], userRemoteConfigs: [[credentialsId: '63a3bb47-63ec-45a8-97db-4e9cbcffb496', url: 'https://github.com/gxglls/go-raft.git']]]) 
	}
	stage ('shell_cmd - Build') {
 			// Shell build step
sh """ 
echo "shell begin"
pwd 
pr1
 """ 
	}
}
}
