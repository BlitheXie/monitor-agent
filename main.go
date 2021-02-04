package main

import (
	"errors"
	"flag"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

func main() {

	configPath := flag.String("config", "/etc/monitor-agent/config.yml", "Config file path")
	port := flag.Int("port", 8080, "Startup port")
	flag.Parse()

	configContent, err := ioutil.ReadFile(*configPath)
	if err != nil {
		panic("Got error while reading config: " + err.Error())
	}
	config := config{}
	err = yaml.Unmarshal(configContent, &config)
	if err != nil {
		panic("Got error while unmarshaling config: " + err.Error())
	}

	prometheusConfigContent, err := ioutil.ReadFile(config.PrometheusAgent.ConfigPath)
	if err != nil {
		panic("Got error while reading prometheus config: " + err.Error())
	}
	prometheusConfig := make(map[interface{}]interface{})
	yaml.Unmarshal([]byte(prometheusConfigContent), prometheusConfig)

	blackboxConfigContent, err := ioutil.ReadFile(config.BlackboxAgent.ConfigPath)
	if err != nil {
		panic("Got error while reading blackbox config: " + err.Error())
	}
	blackboxConfig := make(map[interface{}]interface{})
	yaml.Unmarshal([]byte(blackboxConfigContent), blackboxConfig)
	blackboxConfigModules := blackboxConfig["modules"].(map[string]interface{})

	r := gin.Default()

	r.PUT("/prober", func(c *gin.Context) {
		proberRequest := proberRequest{}
		err := c.BindJSON(&proberRequest)
		if err != nil {
			log.Info("Cannot parse json: ", err.Error())
			c.JSON(400, gin.H{
				"code":    -998,
				"message": "Invalid param",
			})
			return
		}
		log.Info("Upsert prober " + proberRequest.UniqueName)

		proberConfig := createProber(proberRequest)
		blackboxConfigModules[proberRequest.UniqueName] = proberConfig

		saveBlackboxConfig(config.BlackboxAgent, blackboxConfig)
		err = reloadBlackboxConfig(config, c)
		if err != nil {
			log.Error("Cannot reload blackbox config " + err.Error())
		} else {
			log.Info("Successful to reload blackbox config")
		}

	})

	r.DELETE("/prober", func(c *gin.Context) {

		uniqueName := c.Query("uniqueName")
		log.Info("Delete prober config " + uniqueName)
		delete(blackboxConfigModules, uniqueName)

		saveBlackboxConfig(config.BlackboxAgent, blackboxConfig)
		err = reloadBlackboxConfig(config, c)
		if err != nil {
			log.Error("Cannot reload blackbox config " + err.Error())
		} else {
			log.Info("Successful to reload blackbox config")
		}

	})

	r.PUT("/scrapeJob", func(c *gin.Context) {
		scrapeJobRequest := scrapeJobRequest{}
		err := c.BindJSON(&scrapeJobRequest)
		if err != nil {
			log.Info("Cannot parse json: ", err.Error())
			c.JSON(400, gin.H{
				"code":    -998,
				"message": "Invalid param",
			})
			return
		}
		log.Info("Upsert scrape config " + scrapeJobRequest.UniqueName)

		scrapeConfigs := prometheusConfig["scrape_configs"].([]interface{})
		scrapeConfigIndex := findScrapeConfigIndex(scrapeJobRequest.UniqueName, scrapeConfigs)

		log.Info("scrapeConfigIndex ", scrapeConfigIndex)
		if scrapeConfigIndex != -1 {
			scrapeConfigs[scrapeConfigIndex] = createScrapeConfig(scrapeJobRequest)
		} else {
			scrapeConfigs = append(scrapeConfigs, createScrapeConfig(scrapeJobRequest))
			prometheusConfig["scrape_configs"] = scrapeConfigs
		}

		savePrometheusConfig(config.PrometheusAgent, prometheusConfig)
		err = reloadPrometheusConfig(config, c)
		if err != nil {
			log.Error("Cannot reload prometheus config")
		} else {
			log.Info("Successful to reload prometheus config")
		}

	})

	r.DELETE("/scrapeJob", func(c *gin.Context) {

		uniqueName := c.Query("uniqueName")
		log.Info("Delete scrape config " + uniqueName)
		scrapeConfigs := prometheusConfig["scrape_configs"].([]interface{})
		scrapeConfigIndex := findScrapeConfigIndex(uniqueName, scrapeConfigs)

		log.Info("scrapeConfigIndex ", scrapeConfigIndex)
		if scrapeConfigIndex == -1 {
			log.Info("No such scrape config ", uniqueName)
			c.JSON(200, gin.H{
				"code":    0,
				"message": "Success",
			})
			return
		}

		deleteScrapeConfig(scrapeConfigs, prometheusConfig, scrapeConfigIndex)

		savePrometheusConfig(config.PrometheusAgent, prometheusConfig)
		err = reloadPrometheusConfig(config, c)
		if err != nil {
			log.Error("Cannot reload prometheus config")
		} else {
			log.Info("Successful to reload prometheus config")
		}

	})

	r.Run(":" + strconv.Itoa(*port))
}

type config struct {
	PrometheusAgent prometheusAgent `yaml:"prometheusAgent"`
	BlackboxAgent   blackboxAgent   `yaml:"blackboxAgent"`
}

type prometheusAgent struct {
	ConfigPath     string `yaml:"configPath"`
	ReloadEndpoint string `yaml:"reloadEndpoint"`
}

type blackboxAgent struct {
	ConfigPath     string `yaml:"configPath"`
	ReloadEndpoint string `yaml:"reloadEndpoint"`
}

type scrapeJobRequest struct {
	UniqueName    string
	TargetUrls    []string
	Prober        string
	Env           string
	SystemAlertID string
}

type proberRequest struct {
	UniqueName string
	Method     string
	BasicAuth  proberRequestBasicAuth
	Body       string
	Headers    map[string]string
}

type proberRequestBasicAuth struct {
	Username string
	Password string
}

func createProber(proberRequest proberRequest) map[string]interface{} {
	proberHTTPConfig := map[string]interface{}{
		"method": proberRequest.Method,
	}
	if len(proberRequest.Headers) != 0 {
		proberHTTPConfig["headers"] = proberRequest.Headers
	}
	if proberRequest.Body != "" {
		proberHTTPConfig["body"] = proberRequest.Body
	}

	if proberRequest.BasicAuth.Username != "" && proberRequest.BasicAuth.Password != "" {
		proberHTTPBasicAuthConfig := map[string]interface{}{
			"username": proberRequest.BasicAuth.Username,
			"password": proberRequest.BasicAuth.Password,
		}
		proberHTTPConfig["basic_auth"] = proberHTTPBasicAuthConfig
	}

	proberHTTPTLSConfig := map[string]interface{}{
		"insecure_skip_verify": true,
	}
	proberHTTPConfig["tls_config"] = proberHTTPTLSConfig

	proberConfig := map[string]interface{}{
		"prober": "http",
		"http":   proberHTTPConfig,
	}
	return proberConfig
}

func saveBlackboxConfig(blackboxAgent blackboxAgent, blackboxConfig map[interface{}]interface{}) {
	file, _ := os.OpenFile(blackboxAgent.ConfigPath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0664)
	defer file.Close()
	enc := yaml.NewEncoder(file)
	enc.SetIndent(2)
	enc.Encode(blackboxConfig)
}

func reloadBlackboxConfig(config config, c *gin.Context) error {
	reloadRes, err := http.PostForm(config.BlackboxAgent.ReloadEndpoint, url.Values{})
	if err != nil {
		c.JSON(500, gin.H{
			"code":    -999,
			"message": "System error",
		})
		return errors.New("Failed to trigger reloading of blackbox config " + err.Error())
	}
	defer reloadRes.Body.Close()
	log.Info("reloadRes http code" + strconv.Itoa(reloadRes.StatusCode))

	if reloadRes.StatusCode == 200 {
		log.Info("Successful to trigger reloading of blackbox config")
		c.JSON(200, gin.H{
			"code":    0,
			"message": "Success",
		})
		return nil
	}

	reloadResBody, err := ioutil.ReadAll(reloadRes.Body)
	if err != nil {
		c.JSON(500, gin.H{
			"code":    -999,
			"message": "System error",
		})
		return errors.New("Failed to read reloadResBody " + err.Error())
	}
	c.JSON(500, gin.H{
		"code":    -999,
		"message": "System error",
	})
	return errors.New("Failed to trigger reloading of blackbox config " + string(reloadResBody))
}

func createScrapeConfig(req scrapeJobRequest) map[string]interface{} {
	params := map[string]interface{}{
		"module": []string{req.Prober},
	}
	scrapeConfig := map[string]interface{}{
		"job_name":     req.UniqueName,
		"metrics_path": "/probe",
		"params":       params,
	}

	labels := map[string]interface{}{
		"env":             req.Env,
		"system_alert_id": req.SystemAlertID,
	}
	staticConfig := map[string]interface{}{
		"targets": req.TargetUrls,
		"labels":  labels,
	}
	staticConfig["targets"] = req.TargetUrls
	staticConfigs := [1]map[string]interface{}{staticConfig}
	scrapeConfig["static_configs"] = staticConfigs

	relabelConfigs := [3]interface{}{}
	relabelConfigs[0] = map[string]interface{}{
		"source_labels": []string{"__address__"},
		"target_label":  "__param_target",
	}
	relabelConfigs[1] = map[string]interface{}{
		"source_labels": []string{"__param_target"},
		"target_label":  "instance",
	}
	relabelConfigs[2] = map[string]interface{}{
		"target_label": "__address__",
		"replacement":  "127.0.0.1:9115",
	}
	scrapeConfig["relabel_configs"] = relabelConfigs
	return scrapeConfig
}

func deleteScrapeConfig(scrapeConfigs []interface{}, prometheusConfig map[interface{}]interface{}, index int) {
	if len(scrapeConfigs) == index+1 {
		prometheusConfig["scrape_configs"] = scrapeConfigs[:index]
	} else if index == 0 {
		prometheusConfig["scrape_configs"] = scrapeConfigs[1:]
	} else {
		prometheusConfig["scrape_configs"] = append(scrapeConfigs[:index], scrapeConfigs[index+1:]...)
	}
}

func findScrapeConfigIndex(scrapeJobUniqueName string, scrapeConfigs []interface{}) int {
	scrapeConfigIndex := -1
	for i, scrapeConfig := range scrapeConfigs {
		switch scrapeConfig.(type) {
		case map[interface{}]interface{}:
			{
				if scrapeConfig.(map[interface{}]interface{})["job_name"] == scrapeJobUniqueName {
					scrapeConfigIndex = i
					break
				}
			}
		case map[string]interface{}:
			{
				if scrapeConfig.(map[string]interface{})["job_name"] == scrapeJobUniqueName {
					scrapeConfigIndex = i
					break
				}
			}
		default:
		}
	}
	return scrapeConfigIndex
}

func savePrometheusConfig(prometheusAgent prometheusAgent, prometheusConfig map[interface{}]interface{}) {
	file, _ := os.OpenFile(prometheusAgent.ConfigPath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0664)
	defer file.Close()
	enc := yaml.NewEncoder(file)
	enc.SetIndent(2)
	enc.Encode(prometheusConfig)
}

func reloadPrometheusConfig(config config, c *gin.Context) error {
	reloadRes, err := http.PostForm(config.PrometheusAgent.ReloadEndpoint, url.Values{})
	if err != nil {
		c.JSON(500, gin.H{
			"code":    -999,
			"message": "System error",
		})
		return errors.New("Failed to trigger reloading of prometheus config " + err.Error())
	}
	defer reloadRes.Body.Close()
	log.Info("reloadRes http code" + strconv.Itoa(reloadRes.StatusCode))

	if reloadRes.StatusCode == 200 {
		c.JSON(200, gin.H{
			"code":    0,
			"message": "Success",
		})
		return errors.New("Successful to trigger reloading of prometheus config")
	}

	reloadResBody, err := ioutil.ReadAll(reloadRes.Body)
	if err != nil {
		log.Error("Failed to read reloadResBody " + err.Error())
		c.JSON(500, gin.H{
			"code":    -999,
			"message": "System error",
		})
		return nil
	}
	c.JSON(500, gin.H{
		"code":    -999,
		"message": "System error",
	})
	return errors.New("Failed to trigger reloading of prometheus config " + string(reloadResBody))
}
