# Prometheus和Blackbox Exporter的配置生成工具  
## 简介  
公司用Prometheus和Blackbox Exporter来监控系统健康状况，目前只监控http，由于添加新的监控目标需要直接进入机器修改yml文件，而监控的系统又比较多，涉及多个部门，太多人修改yml文件容易造成混乱，所以打造了Monitor Dashboard(Java) + Monitor Agent(Go)的配置管理平台  
之所以要采用Dashboard+Agent的架构:  
1. Dashboard的机器和Monitor(Prometheus, Blackbox Exporter)的机器是分开的，如果Dashboard需要直接修改yml，则要能够ssh过去，这样会有安全隐患，况且以后搭建Monitor集群可能会存在某些Monitor机器无法直接ssh过去，而http就不存在这种问题  
2. Agent使用与Prometheus和Blackbox Exporter相同的yml文件解析库，避免由于库的不同而导致解析失败  
3. Agent屏蔽修改yml文件细节，只需提供必要的参数，便于解耦  
4. Monitor机器配置较低，而Go资源占用比Java低，写Agent有优势  
## 使用
默认配置文件/etc/monitor-agent/config.yml，可用`--config`指定
默认端口8080，可用`--port`指定

