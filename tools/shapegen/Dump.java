import com.mojang.blaze3d.vertex.PoseStack;
import com.mojang.math.Transformation;
import java.lang.reflect.Method;
import java.util.Locale;
import net.minecraft.client.model.geom.ModelPart;
import net.minecraft.client.model.geom.builders.LayerDefinition;
import net.minecraft.core.Direction;
import org.joml.Vector3f;

// Dump bakes a LayerDefinition from the game and prints every cube face as
// JSON: part path, cube index, normal and each vertex as its position in
// block pixels and its UV (0..1), placed by the renderer's own transform.
// Args: modelClass factory [xfClass xfMethod (dir <name> | int <n> | float <f>)]
public class Dump {
    public static void main(String[] a) throws Exception {
        Method f = Class.forName(a[0]).getDeclaredMethod(a[1]);
        f.setAccessible(true);
        ModelPart root = ((LayerDefinition) f.invoke(null)).bakeRoot();
        PoseStack ps = new PoseStack();
        if (a.length > 2) {
            Class<?> argType = switch (a[4]) { case "dir" -> Direction.class; case "int" -> int.class; default -> float.class; };
            Object arg = switch (a[4]) { case "dir" -> Direction.valueOf(a[5].toUpperCase(Locale.ROOT)); case "int" -> Integer.parseInt(a[5]); default -> Float.parseFloat(a[5]); };
            Method x = Class.forName(a[2]).getDeclaredMethod(a[3], argType);
            x.setAccessible(true);
            ps.mulPose((Transformation) x.invoke(null, arg));
        }
        StringBuilder out = new StringBuilder("[");
        root.visit(ps, (pose, path, index, cube) -> {
            for (ModelPart.Polygon p : cube.polygons) {
                Vector3f n = pose.transformNormal(new Vector3f(p.normal()), new Vector3f());
                out.append(String.format(Locale.ROOT, "{\"part\":\"%s\",\"cube\":%d,\"normal\":[%.4f,%.4f,%.4f],\"verts\":[", path, index, n.x, n.y, n.z));
                for (ModelPart.Vertex v : p.vertices()) {
                    Vector3f w = pose.pose().transformPosition(new Vector3f(v.x() / 16f, v.y() / 16f, v.z() / 16f));
                    out.append(String.format(Locale.ROOT, "[%.4f,%.4f,%.4f,%.6f,%.6f],", w.x * 16, w.y * 16, w.z * 16, v.u(), v.v()));
                }
                out.setLength(out.length() - 1);
                out.append("]},");
            }
        });
        if (out.length() > 1) out.setLength(out.length() - 1);
        System.out.println(out.append("]"));
    }
}
