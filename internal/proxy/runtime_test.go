package proxy

import "testing"

func TestStatsDistinguishesMaintenanceDownAndServing(t *testing.T) {
	targets, err := ParseTargets("# pxname,svname,status,scur,\ndemo_servers,primary,UP,3,\ndemo_servers,standby,MAINT,0,\n")
	if err != nil {
		t.Fatal(err)
	}
	if !targets["primary"].Healthy || targets["primary"].Sessions != 3 || targets["standby"].Enabled {
		t.Fatalf("incorrect stats: %+v", targets)
	}
	for _, body := range []string{"permission denied", "# pxname,svname,status,scur\ndemo_servers,primary,UP,0\n", "# pxname,svname,status,scur\ndemo_servers,primary,UP,invalid\ndemo_servers,standby,DOWN,0\n"} {
		if _, err := ParseTargets(body); err == nil {
			t.Fatalf("accepted bad proxy response: %s", body)
		}
	}
}
